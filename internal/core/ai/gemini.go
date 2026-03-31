package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"strconv"


	"github.com/google/generative-ai-go/genai"
	"github.com/ledaian41/verix-stock/internal/modules/article"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"
)



type Synthesizer struct {
	client                 *genai.Client
	factModel              *genai.GenerativeModel
	proSynthesisModel      *genai.GenerativeModel
	fallbackSynthesisModel *genai.GenerativeModel
	rpmLimit               int
	limiter                *rate.Limiter
}



func NewSynthesizer(ctx context.Context) (*Synthesizer, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	limitStr := os.Getenv("GEMINI_RPM_LIMIT")
	rpm, _ := strconv.Atoi(limitStr)
	if rpm <= 0 {
		rpm = 12 // Default safe limit for Free Tier (15 RPM)
	}



	// 1. Fact Extraction Model (Flash 2.0)
	factModel := client.GenerativeModel("gemini-2.0-flash")
	factModel.SetTemperature(0.1)
	factModel.ResponseMIMEType = "application/json"
	factModel.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"ticker":     {Type: genai.TypeString},
			"article_id": {Type: genai.TypeInteger},
			"numbers":    {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			"events":     {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			"risks":      {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			"signal":     {Type: genai.TypeString, Enum: []string{"positive", "neutral", "negative"}},
		},
		Required: []string{"ticker", "article_id", "numbers", "events", "risks", "signal"},
	}

	// Synthesis Result Schema (Shared)
	synthesisSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"summary":         {Type: genai.TypeString, Description: "3-5 gạch đầu dòng ý chính, mỗi ý 1 dòng"},
			"conclusion":      {Type: genai.TypeString, Description: "1 câu kết luận ngắn gọn, không kèm icon"},
			"sentiment_score": {Type: genai.TypeNumber},
		},
		Required: []string{"summary", "conclusion", "sentiment_score"},
	}

	// 2. High Quality Synthesis Model (Pro 1.5)
	proSynthesisModel := client.GenerativeModel("gemini-1.5-pro")
	proSynthesisModel.SetTemperature(0.1)
	proSynthesisModel.ResponseMIMEType = "application/json"
	proSynthesisModel.ResponseSchema = synthesisSchema

	// 3. Fallback Synthesis Model (Flash 2.0)
	fallbackSynthesisModel := client.GenerativeModel("gemini-2.0-flash")
	fallbackSynthesisModel.SetTemperature(0.1)
	fallbackSynthesisModel.ResponseMIMEType = "application/json"
	fallbackSynthesisModel.ResponseSchema = synthesisSchema

	return &Synthesizer{
		client:                 client,
		factModel:              factModel,
		proSynthesisModel:      proSynthesisModel,
		fallbackSynthesisModel: fallbackSynthesisModel,
		rpmLimit:               rpm,
		limiter:                rate.NewLimiter(rate.Every(time.Minute/time.Duration(rpm)), 1),
	}, nil
}




func (s *Synthesizer) Close() {
	s.client.Close()
}

type FactResult struct {
	Ticker    string   `json:"ticker"`
	ArticleID uint     `json:"article_id"`
	Numbers   []string `json:"numbers"`
	Events    []string `json:"events"`
	Risks     []string `json:"risks"`
	Signal    string   `json:"signal"`
}

type SynthesisResult struct {
	Summary        string
	Conclusion     string
	SentimentScore float64
}

// Synthesize implements the smart routing logic
func (s *Synthesizer) Synthesize(ctx context.Context, ticker string, drafts []article.DraftArticle) (*SynthesisResult, error) {
	if len(drafts) <= 3 {
		return s.synthesizeDirect(ctx, ticker, drafts)
	}

	// For legacy compatibility or small batches > 3, we still do two-stage here but sequentially
	var facts []FactResult
	for _, d := range drafts {
		f, err := s.ExtractFact(ctx, d)
		if err != nil {
			return nil, err
		}
		facts = append(facts, *f)
	}

	return s.SynthesizeFromFacts(ctx, ticker, facts)
}


func (s *Synthesizer) synthesizeDirect(ctx context.Context, ticker string, drafts []article.DraftArticle) (*SynthesisResult, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Hành động như một chuyên gia phân tích tài chính cao cấp. Hãy tóm tắt các tin tức mới nhất về mã cổ phiếu %s.\n\n", ticker))
	sb.WriteString("Yêu cầu:\n")
	sb.WriteString("1. Trường 'summary': Viết 3-5 gạch đầu dòng cô đọng, chuyên nghiệp.\n")
	sb.WriteString("2. Trường 'conclusion': Viết 1 câu chốt ngắn gọn đại diện cho toàn bộ tin tức (không bao gồm emoji 📌).\n")
	sb.WriteString("3. Đưa ra điểm Sentiment từ -1.0 đến 1.0.\n")
	sb.WriteString("4. Định dạng phản hồi: JSON.\n\n")

	for i, d := range drafts {
		sb.WriteString(fmt.Sprintf("Tin %d: %s\nNội dung: %s\n\n", i+1, d.Title, d.FullContent))
	}

	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	resp, err := s.proSynthesisModel.GenerateContent(ctx, genai.Text(sb.String()))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		resp, err = s.fallbackSynthesisModel.GenerateContent(ctx, genai.Text(sb.String()))
		if err != nil {
			return nil, err
		}
	}


	return s.parseResponse(resp), nil
}

func (s *Synthesizer) SynthesizeFromFacts(ctx context.Context, ticker string, facts []FactResult) (*SynthesisResult, error) {
	// Stage 2: Synthesis with Conflict Handling

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Dưới đây là dữ liệu thô đã trích xuất từ %d bài báo về mã %s.\n", len(facts), ticker))
	sb.WriteString("Hãy hành động như một chuyên gia phân tích tài chính để tổng hợp thành bản tin cuối cùng.\n\n")
	sb.WriteString("QUY TẮC XỬ LÝ MÂU THUẪN:\n")
	sb.WriteString("- Nếu các nguồn tin đưa ra số liệu hoặc nhận định trái ngược nhau, BẮT BUỘC phải nêu rõ sự đối lập (Ví dụ: 'Mặc dù A... nhưng B...').\n")
	sb.WriteString("- Ưu tiên các dữ liệu có con số cụ thể.\n\n")
	sb.WriteString("Yêu cầu đầu ra:\n")
	sb.WriteString("1. Trường 'summary': 3-5 gạch đầu dòng tóm tắt tinh hoa.\n")
	sb.WriteString("2. Trường 'conclusion': 1 câu kết luận cô đọng cuối cùng (không bao gồm emoji 📌).\n")
	sb.WriteString("3. Điểm Sentiment tổng hợp (-1.0 đến 1.0).\n")
	sb.WriteString("4. Định dạng phản hồi: JSON.\n\n")

	factsJSON, err := json.Marshal(facts) // Use compact JSON to save tokens
	if err != nil {
		return nil, fmt.Errorf("failed to marshal facts: %w", err)
	}
	sb.WriteString("DỮ LIỆU TRÍCH XUẤT:\n")
	sb.WriteString(string(factsJSON))

	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	resp, err := s.proSynthesisModel.GenerateContent(ctx, genai.Text(sb.String()))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		resp, err = s.fallbackSynthesisModel.GenerateContent(ctx, genai.Text(sb.String()))
		if err != nil {
			return nil, err
		}
	}


	return s.parseResponse(resp), nil
}

func (s *Synthesizer) ExtractFact(ctx context.Context, d article.DraftArticle) (*FactResult, error) {

	prompt := fmt.Sprintf(`Phân tích bài báo về mã %s. 
Tiêu đề: %s
Trích xuất:
- numbers: CHỈ các con số có giá trị phân tích (%%, tỷ đồng, EPS, PE...). Bỏ ngày tháng thuần túy.
- events: Sự kiện cụ thể, có thể ảnh hưởng giá (M&A, kết quả kinh doanh, hợp đồng lớn...).
- risks: Rủi ro được đề cập rõ ràng trong bài.
- signal: Tone chủ đạo của bài (positive/neutral/negative).

Nội dung bài báo: %s`, d.Ticker, d.Title, d.FullContent)
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	resp, err := s.factModel.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("factModel error: %v", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty factModel candidates")
	}

	var result FactResult
	partContent := resp.Candidates[0].Content.Parts[0]
	text, ok := partContent.(genai.Text)
	if !ok {
		return nil, fmt.Errorf("factModel response part 0 is not text: %v", partContent)
	}
	data := []byte(text)
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fact response (%s): %v", string(data), err)
	}

	result.ArticleID = d.ID // Ensure ID is correct
	return &result, nil
}

func (s *Synthesizer) parseResponse(resp *genai.GenerateContentResponse) *SynthesisResult {
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return &SynthesisResult{Summary: "No content generated", SentimentScore: 0}
	}

	part := resp.Candidates[0].Content.Parts[0]
	text, ok := part.(genai.Text)
	if !ok {
		return &SynthesisResult{Summary: "Unexpected response format", SentimentScore: 0}
	}

	var result struct {
		Summary        string  `json:"summary"`
		Conclusion     string  `json:"conclusion"`
		SentimentScore float64 `json:"sentiment_score"`
	}

	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// Fallback for non-JSON or partial JSON
		return &SynthesisResult{Summary: string(text), SentimentScore: 0}
	}

	return &SynthesisResult{
		Summary:        result.Summary,
		Conclusion:     result.Conclusion,
		SentimentScore: result.SentimentScore,
	}
}
