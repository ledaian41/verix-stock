# Ý tưởng chính của dự án Verix Stock

## 1. Phân tích đối tượng người dùng (User Persona: F0)
- **Nỗi đau (Pain points)**: Quá nhiều nguồn tin rác, không có thời gian theo dõi bảng điện liên tục, khó hiểu các thuật ngữ tài chính phức tạp.
- **Nhu cầu**: Cảnh báo tức thì về mã đang giữ, tóm tắt tin tức bằng ngôn ngữ dễ hiểu (AI Summary), giao diện tối giản (Telegram-first).

## 2. Kiến trúc Hệ thống (System Architecture)

### A. Tầng Thu thập & Xử lý (The Engine - Go CLI Worker)
- **Cơ chế**: Quét đa luồng (Goroutines) các đầu báo chính thống. 
    - **Deep Crawl**: Truy cập vào từng link bài viết để lấy **Toàn văn (Full Content)** thay vì chỉ lấy đoạn trích dẫn ngắn (Sapo), giúp AI có dữ liệu chất lượng cao.
- **AI Integration**: Sử dụng Gemini API để **Tổng hợp (Synthesize)** các tin tức trong cùng một phiên (Sáng/Chiều) của cùng một mã cổ phiếu thành một bản tin duy nhất (3-5 gạch đầu dòng) kèm điểm số Sentiment.
- **Deduplication & Optimization**: 
    - **Transient Drafts**: Lưu tin thô vào bảng `DraftArticle`, sau khi AI tổng hợp xong sẽ xóa sạch để tối ưu database.
    - **Crawler Metadata**: Sử dụng bảng metadata riêng để lưu mốc thời gian cào tin (`last_crawled_at`), giúp bảng Draft luôn trống và truy vấn cực nhanh.
- **Scheduling**: Sử dụng `robfig/cron` (v3) để quản lý:
    - 08:00 & 15:00: Cào tin mới.
    - 08:15 & 15:15: AI tổng hợp và gửi cảnh báo.
    - 00:00: Dọn dẹp tin cũ (giữ lại 1 năm).

### B. Tầng Quản trị & Auth (The Admin Host - Go API)
- **Auth**: Sử dụng Telegram Login Widget. Đây là cách tối ưu cho F0 vì họ không cần nhớ mật khẩu, chỉ cần một nút nhấn là xong.
- **Config UI**: API host trả về file HTML đơn giản để User quản lý Watchlist (Thêm/Xóa mã SSI, HPG...).
- **Endpoint Public**: Cung cấp JSON cho Frontend Next.js lấy danh sách tin tức đã được xử lý.

### C. Tầng Hiển thị & Cảnh báo (The Frontend & Bot)
- **Frontend (Next.js)**: Trang tin tức công khai, tối ưu SEO, hiển thị danh sách tin kèm điểm số tâm lý từ AI.
- **Telegram Bot**: Đóng vai trò là kênh "Push Notification". Khi Worker quét thấy tin có `sentiment_score` biến động mạnh (ví dụ tin rất xấu), nó sẽ tra cứu bảng Watchlist và gửi tin nhắn đồng loạt cho tất cả User đang theo dõi mã đó.

## 3. Sơ đồ luồng dữ liệu (Data Flow)
1. **Worker (Local)**: Quét tin $\rightarrow$ AI Tóm tắt $\rightarrow$ Lưu Supabase.
2. **User (Web)**: Login Telegram $\rightarrow$ Lưu mã HPG vào Watchlist trên API Host.
3. **Hệ thống (Trigger)**: Worker thấy tin HPG mới $\rightarrow$ Kiểm tra ai đang Watch HPG $\rightarrow$ Gọi API Telegram gửi tin nhắn.
4. **Frontend (Next.js)**: Fetch data từ API Host để hiển thị Timeline tin tức tổng hợp cho cộng đồng.

## 4. Ưu điểm của mô hình "Chia sẻ nguồn tin"
- **Tiết kiệm chi phí**: Chỉ tốn 1 lần gọi AI cho 1 bài báo, dù có 1.000 user đang theo dõi mã đó.
- **Tốc độ**: Vì tin tức được xử lý tập trung, việc gửi thông báo Telegram gần như tức thời (Real-time).
- **Tính cộng đồng**: Frontend Public tạo ra một giá trị chung cho mọi người, thu hút user mới mà không cần bắt họ đăng ký ngay.
