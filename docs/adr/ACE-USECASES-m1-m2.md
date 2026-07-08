# ACE Platform — Use Case & Flow Specification: Module 1 (Supply) & Module 2 (Package)

> Nguồn: `RFP_ACE_NEWFORMAT_v2.0_FINAL.pdf`, mục 4.1 (Module 1) và 4.2 (Module 2).
> Tài liệu này mô tả **use case và luồng nghiệp vụ** theo góc nhìn actor. Các quyết định kiến trúc,
> DB schema, DDL và cơ chế reliability (outbox, snapshot bất biến...) đã được phân tích trong
> [`ACE-ADR-0002-m1-m2.md`](./ACE-ADR-0002-m1-m2.md) — tài liệu này **không lặp lại** phần đó,
> chỉ tham chiếu khi cần giải thích luồng.

---

## 1. Tổng quan

| Module | Tên | Vai trò |
|--------|-----|---------|
| M1 | Service & Supply Aggregation | Thu gom, chuẩn hoá dịch vụ từ nhà cung ứng thành entitlement bán được |
| M2 | Package & Experience Configuration Engine | Đóng gói entitlement thành sản phẩm bán được, định giá, targeting |

**Actor tổng hợp (M1 + M2):**

| Actor | Xuất hiện ở | Vai trò chính |
|-------|-------------|----------------|
| Nhà cung ứng Admin | M1 | Đăng ký, quản lý dịch vụ, giá, inventory |
| Nhà cung ứng Sub Account | M1 | Operator/Viewer — thao tác giới hạn theo phân quyền |
| ACE Admin | M1, M2 | Phê duyệt supplier, service, package; kiểm soát toàn bộ pipeline thương mại |
| Content Manager (ACE) | M1, M2 | Quản lý nội dung đa ngôn ngữ (dịch vụ và gói) |
| Finance Manager (ACE) | M1, M2 | Cấu hình giá, revenue split, kiểm soát biên lợi nhuận |
| Product Manager | M1 (đọc), M2 (chính) | Xây dựng, định giá, publish gói dịch vụ |
| Airport NEO Operator | M1 | Giám sát SLA kỹ thuật nhà cung cấp |
| ACE Operator | M2 | Vận hành, giám sát danh sách gói |
| B2B Customer (Bank/Corporate) | M2 | Duyệt/mua gói bulk qua B2B portal (giao dịch thực tại M6) |
| System / External System (API) | M1, M2 | Đồng bộ inventory, engine targeting/pricing/A-B test |

Quy ước sơ đồ: ASCII box/arrow trong code block, nhất quán với `ACE-ADR-0002-m1-m2.md` /
`ACE-ADR-0003-m6-m7.md` (không dùng Mermaid).

---

## 2. Module 1 — Service & Supply Aggregation

### 2.1 Actor (RFP 4.1.2)

Nhà cung ứng Admin, Nhà cung ứng Sub Account, ACE Admin, Content Manager (ACE), Finance Manager
(ACE), Product Manager (đọc catalog), Airport NEO Operator, System (API) / External System.

### 2.2 EPIC M1-E1 — Supplier/Partner Onboarding & Management

| ID | Actor | Use case |
|----|-------|----------|
| M1-US-01 | ACE Admin, Nhà cung ứng | Đăng ký nhà cung ứng mới, digital onboarding + KYC cơ bản |
| M1-US-02 | Nhà cung ứng Admin | Quản lý hồ sơ công ty và tài khoản ngân hàng |
| M1-US-03 | Nhà cung ứng Admin | Quản lý vai trò user trong tài khoản nhà cung ứng (admin/operator/viewer) |

**Flow — M1-US-01: Onboarding nhà cung ứng**

```
 Nhà cung ứng                        Hệ thống (M1)                  ACE Admin
     │                                    │                              │
     │ 1. Điền form online: tên, loại     │                              │
     │  hình dịch vụ, MST, ngân hàng,     │                              │
     │  contact                           │                              │
     ├────────────────────────────────────▶                              │
     │                                    │ 2. Validate MST (checksum,   │
     │                                    │  không trùng lặp), chống     │
     │                                    │  spam (captcha/rate-limit)   │
     │                                    │  status = pending_profile    │
     │                                    │  → pending_review            │
     │                                    ├─────────────────────────────▶│
     │                                    │            3. Review KYC     │
     │                                    │            trên UI quản trị  │
     │                                    │◀──────────────────────────────┤
     │                                    │  4a. Approve → status=active │
     │                                    │  4b. Reject/thiếu hồ sơ →    │
     │                                    │      status=needs_update     │
     │◀────────────────────────────────────┤                              │
     │ 5. Email tự động: client_id +      │                              │
     │    client_secret (không trao đổi   │                              │
     │    thủ công)                       │                              │
     ▼                                    ▼                              ▼
  Đăng nhập portal, bắt đầu khai báo dịch vụ  ────────────────────▶  EPIC M1-E2
```

Quy tắc: MST không được sửa sau KYC thành công; đổi thông tin ngân hàng bắt buộc OTP 2 bước và
cần ACE Admin phê duyệt lại (trạng thái pending riêng cho thay đổi ngân hàng); mọi thao tác ghi
audit log.

**Sub-flow — M1-US-03: phân quyền trong tài khoản nhà cung ứng**

```
 Nhà cung ứng Admin (full quyền)
        │
        ├──▶ tạo/ vô hiệu hoá user con (không xoá — giữ lịch sử)
        │
        ├──▶ Operator  — upload/ chỉnh sửa dịch vụ, inventory
        └──▶ Viewer    — chỉ đọc

 Mọi hành động của user con → audit log (ai, khi nào, việc gì)
```

### 2.3 EPIC M1-E2 — Service Catalog Management

| ID | Actor | Use case |
|----|-------|----------|
| M1-US-04 | Nhà cung ứng, ACE Admin | Tạo & chuẩn hoá dịch vụ thành entitlement unit chuẩn |
| M1-US-05 | ACE Admin / Nhà cung ứng / PM | Cấu hình giá dịch vụ (static pricing, tier, bundle override) |
| M1-US-06 | Content Manager | Quản lý nội dung đa ngôn ngữ (VI/EN) |
| M1-US-07 | ACE Admin | Vô hiệu hoá (pause) / ngừng cung cấp (archive) dịch vụ |

**Flow — M1-US-04 + M1-US-05: tạo dịch vụ → định giá → phê duyệt → snapshot**

```
 Nhà cung ứng                     Hệ thống (M1)                    ACE Admin / Content Mgr
     │                                  │                                  │
     │ 1. Upload dịch vụ: tên, mô tả    │                                  │
     │  (VI/EN), ảnh, T&C, giá base,    │                                  │
     │  entitlement_unit                │                                  │
     │  (lounge_minute/fast_track_token │                                  │
     │  /dining_credit/limo_voucher/    │                                  │
     │  baggage_unit)     status=draft  │                                  │
     ├──────────────────────────────────▶ 2. Chuẩn hoá sang schema         │
     │                                  │  entitlement nội bộ, validate    │
     │ 3. Cấu hình pricing: base price, │                                  │
     │  discount tier theo volume,      │                                  │
     │  bundle override, effective_from │                                  │
     ├──────────────────────────────────▶                                  │
     │ 4. Submit review                 │                                  │
     │  status → pending_review         │                                  │
     ├──────────────────────────────────┼─────────────────────────────────▶│
     │                                  │              5. Review nội dung, │
     │                                  │              giá, hình ảnh       │
     │                                  │◀─────────────────────────────────┤
     │                        ┌─────────┴─────────┐                        │
     │                        ▼                    ▼                       │
     │                    approve               reject                     │
     │           status=approved          status=needs_update              │
     │           snapshot v1 tạo          (nhà cung ứng sửa & submit lại)  │
     │           (bất biến — chi tiết                                      │
     │           ở ADR-0002 D2)                                            │
     │                        │                                            │
     │                        ▼                                            │
     │                status=active — hiển thị trên catalog                │
     │                (tìm kiếm theo airport_code/category/                │
     │                entitlement_unit/status; export CSV)                 │
     │                        │                                            │
     │                        ▼                                            │
     │        M2 (Product Manager) đọc snapshot qua API khi cần            │
     │        xây dựng gói — KHÔNG cần Kafka event (xem §4 và ADR-0002)    │
```

Quy tắc quan trọng:
- Sau khi entitlement đã nằm trong ≥1 package active hoặc đã có voucher issued, mọi sửa đổi tạo
  snapshot mới (v2, v3…); voucher đã issued luôn hiển thị theo snapshot tại thời điểm issued.
- Khi giá thay đổi (M1-US-05): đơn đã tạo giữ giá cũ; **mọi package M2 đang hiển thị bán liên quan
  tự động chuyển về `pending_review`** để tránh bán sai giá (package đã bán giữ nguyên) — đây là
  cascade `price.changed` (Kafka, bắt buộc — xem §5, ADR-0002 D5).

**Sub-flow — M1-US-07: pause/archive (một chiều)**

```
  active ──▶ paused ──▶ archived
   │           │            │
   │           │            └─ không thể revert; release toàn bộ inventory
   │           │               chưa dùng; không nhận đơn mới; đơn đã có
   │           │               vẫn fulfill theo cam kết
   │           └─ ẩn khỏi marketplace; đơn đang pending giữ nguyên
   │
   └─ cảnh báo hiển thị nếu dịch vụ đang có đơn/inventory chưa fulfill
      trước khi cho phép pause/archive
```

**Sub-flow — M1-US-06: nội dung đa ngôn ngữ**

```
  service_id
     ├── content.vi  (bắt buộc)
     └── content.en  (fallback → vi nếu chưa cấu hình)

  Content Manager / ACE Admin chỉnh sửa 2 tab song song (vi/en)
  → version control (lịch sử + rollback) → phản ánh gần real-time (≤5s)
  ra marketplace
```

### 2.4 EPIC M1-E3 — Inventory Management

| ID | Actor | Use case |
|----|-------|----------|
| M1-US-08 | Nhà cung ứng Admin | Quản lý tồn kho theo timeslot, cập nhật capacity realtime |
| M1-US-09 | Nhà cung ứng | Bulk upload inventory qua CSV/Excel |
| M1-US-10 | ACE Admin | Alert khi inventory sắp hết |

**Flow — M1-US-08: giữ chỗ, atomic update, waitlist, auto-release**

```
 Nhà cung ứng                 Hệ thống (M1 Inventory)              Order (M6, qua Kafka/API)
     │                              │                                     │
     │ 1. Định nghĩa capacity theo  │                                     │
     │  timeslot (vd 50 lượt/giờ)   │                                     │
     │  overbook_pct 0–20%          │                                     │
     ├──────────────────────────────▶                                     │
     │                              │◀────────────────────────────────────┤
     │                              │   2. order.confirmed                │
     │                              │   atomic UPDATE capacity_used       │
     │                              │   += qty WHERE capacity_used+qty    │
     │                              │   <= capacity_total*(1+overbook_pct)│
     │                              │                                     │
     │                    3a. đủ chỗ → reservation(held), TTL 15 phút     │
     │                    3b. hết chỗ → waitlist theo thứ tự, notify khi  │
     │                        có slot trống                               │
     │                              │                                     │
     │                    4. Không xác nhận trong 15p → auto release      │
     │                       capacity_used -= qty, status=released        │
     │                              │                                     │
     │                    5. Dashboard occupancy theo dịch vụ/khung giờ   │
```

**Sub-flow — M1-US-09: bulk upload CSV**

```
  Download template CSV (header chuẩn)
        │
        ▼
  Upload file → validate format/date/capacity>0
        │
        ▼
  Preview: số rows hợp lệ, rows lỗi (kèm lý do)
        │
        ▼
  Confirm submit → ghi hàng loạt vào inventory (idempotent theo request)
```

**Sub-flow — M1-US-10: cảnh báo tồn kho thấp**

```
  Ngưỡng khả dụng (config được) đạt tới
        │
        ▼
  Gửi alert: email + in-app cho ACE Admin
        │
        ▼
  Không lặp lại cảnh báo cho cùng dịch vụ trong 4 giờ (chống spam alert)
```

### 2.5 EPIC M1-E4 — Nhà cung ứng API Integration

| ID | Actor | Use case |
|----|-------|----------|
| M1-US-11 | External System (nhà cung ứng) | REST API đồng bộ inventory/giá qua hệ thống đối tác |
| M1-US-12 | ACE Admin | Giám sát SLA vận hành kỹ thuật nhà cung cấp |
| M1-US-13 | ACE Admin | Role-Based Access Control (RBAC) toàn hệ thống M1 |

**Flow — M1-US-11 + M1-US-12: tích hợp API và giám sát SLA**

```
 Hệ thống Nhà cung ứng (external)      M1 REST API                 M1 Ops / Dashboard
        │                                   │                              │
        │ 1. OAuth 2.0 client_credentials   │                              │
        ├───────────────────────────────────▶                              │
        │◀──────────────── access_token ─────┤                              │
        │                                   │                              │
        │ 2. POST /services/{id}/           │                              │
        │    inventory/sync                 │                              │
        ├───────────────────────────────────▶ 3. Update capacity/giá,      │
        │                                   │    validate, atomic write     │
        │◀──────────── webhook callback ─────┤                              │
        │   (kết quả xử lý, lỗi nếu có)     │                              │
        │                                   │                              │
        │             4. Thu thập metrics: uptime, latency, error rate  ──▶│
        │             5. Vượt ngưỡng SLA → alert email/in-app/Slack        │
        │             6. Sự cố nghiêm trọng → fallback dự phòng tự động    │
```

**Sub-flow — M1-US-13: RBAC**

```
  Role                Quyền
  ──────────────────  ────────────────────────────────────────────
  ACE Admin            Full — tạo/sửa/xoá role, gán quyền, phê duyệt
  Content Manager       CMS (nội dung, hình ảnh) — không phê duyệt giá
  Finance Manager        Pricing, revenue split — không sửa nội dung
  Nhà cung ứng Admin      Dịch vụ, inventory, tài khoản của chính mình
  Nhà cung ứng Operator     Upload/sửa (không xoá, không đổi ngân hàng)
  Nhà cung ứng Viewer         Chỉ đọc

  Thực thi phân quyền ở cả UI (ẩn/khoá control) và API (chặn request)
```

---

## 3. Module 2 — Package & Experience Configuration Engine

### 3.1 Actor (RFP 4.2.2)

Product Manager (chính), ACE Admin (phê duyệt), Finance Manager (revenue split), Content Manager
(nội dung gói), ACE Operator (vận hành), B2B Customer (xem/mua bulk — giao dịch tại M6), Hành
khách Consumer (Phase 2 — thiết kế sẵn sàng từ Phase 1), System/API (targeting, pricing, A/B
test engine).

### 3.2 EPIC M2-E1 — Package Builder (No-code)

| ID | Actor | Use case |
|----|-------|----------|
| M2-US-01 | Product Manager | Tạo gói dịch vụ bằng công cụ kéo-thả no-code |
| M2-US-02 | Product Manager | Gói mẫu (template) theo phân khúc hành khách |
| M2-US-03 | Product Manager | Quy tắc targeting gói theo phân khúc |
| M2-US-04 | Product Manager, ACE Admin | Lifecycle quản lý gói (Draft → Active → Archived) |

**Flow — M2-US-01/02/03/04: xây dựng, targeting, phê duyệt gói**

```
 Product Manager                     Hệ thống (M2)                  ACE Admin
     │                                    │                              │
     │ 1. GET dịch vụ đã approved        │                              │
     │  (airport, category, ...)         │                              │
     ├────────────────────────────────────▶ Pull đồng bộ qua API, KHÔNG   │
     │◀───────────── danh sách snapshot ───┤ cần Kafka — PM chủ động chọn,│
     │                                    │ không cần realtime (§4)      │
     │                                    │                              │
     │ 2. Kéo-thả chọn entitlement, HOẶC  │                              │
     │  chọn từ template có sẵn (Business │                              │
     │  / Family / Solo Traveler...) rồi  │                              │
     │  chỉnh sửa                         │                              │
     ├────────────────────────────────────▶ status=draft (chỉ người tạo   │
     │                                    │ thấy); tự tính giá gói =     │
     │                                    │ tổng giá thành phần + margin │
     │ 3. Cấu hình targeting rules:       │                              │
     │  hạng vé (Y/W/J/C/F), thời gian,   │                              │
     │  sân bay... (AND giữa nhóm, OR     │                              │
     │  trong nhóm); preview hồ sơ giả    │                              │
     │  định để test rule                 │                              │
     ├────────────────────────────────────▶                              │
     │ 4. Submit review                   │                              │
     │  status → pending_review           │                              │
     ├────────────────────────────────────┼─────────────────────────────▶│
     │                                    │              5. Review, kiểm  │
     │                                    │              tra completeness,│
     │                                    │              price sanity     │
     │                                    │◀──────────────────────────────┤
     │                                    │  status = active              │
     │                                    │  package_versions tạo (v1,    │
     │                                    │  bất biến — xem ADR-0002 D3)  │
     │                                    │  event: package.published     │
     │                                    │  (Kafka, cascade tới M6)      │
```

**Sub-flow — M2-US-04: lifecycle & versioning**

```
  draft ──▶ pending_review ──▶ active ──▶ paused
                                    │
                                    └──▶ replaced ──▶ archived

  Sửa package đang active → tạo version mới (draft) song song,
  KHÔNG sửa trực tiếp version đang bán (order_items đã khoá
  package_version_id — xem ADR-0002 D3)
```

### 3.3 EPIC M2-E2 — Pricing & Promotions

| ID | Actor | Use case |
|----|-------|----------|
| M2-US-05 | Product Manager | Cấu hình thời hạn và giá ưu đãi (early-bird, last-minute, flash-sale) |
| M2-US-06 | Finance Manager | Cấu hình phân chia doanh thu (revenue split) |
| M2-US-07 | Product Manager | A/B testing framework cho packages |

**Flow — M2-US-05/06/07**

```
 Product Manager                      Finance Manager                Hệ thống (M2)
     │                                      │                              │
     │ 1. Cấu hình discount %, khoảng thời  │                              │
     │  gian hiệu lực; early-bird (mua sớm  │                              │
     │  → giá thấp hơn); last-minute (mua   │                              │
     │  sát giờ → giá thấp hơn); flash-sale │                              │
     ├──────────────────────────────────────┼─────────────────────────────▶│
     │                                      │ 2. Cấu hình revenue split:   │
     │                                      │  NCC share + platform fee    │
     │                                      │  (5–40%) + ACE margin = 100% │
     │                                      │  (CHECK constraint tại DB —  │
     │                                      │  xem ADR-0002 D7)            │
     │                                      ├──────────────────────────────▶│
     │                                      │  3. Margin âm → cần Finance  │
     │                                      │     Manager phê duyệt riêng   │
     │                                      │  4. Auto áp dụng vào hệ       │
     │                                      │     thống đối soát (M6)       │
     │ 5. Tạo A/B test: variant A/B cho     │                              │
     │  cùng 1 package, khác nhau ở giá/    │                              │
     │  targeting/nội dung                  │                              │
     ├──────────────────────────────────────┼─────────────────────────────▶│
     │                                      │  6. Phân bổ traffic 50/50     │
     │                                      │     (hoặc tuỳ chỉnh)          │
     │                                      │  7. Thu thập metrics: views,  │
     │                                      │     conversion rate, revenue  │
```

### 3.4 EPIC M2-E3 — Multi-channel Display

| ID | Actor | Use case |
|----|-------|----------|
| M2-US-08 | Product Manager | Cấu hình hiển thị gói theo channel (app/web/B2B portal/kiosk) |
| M2-US-09 | B2B Customer (Bank/Corporate) | Duyệt & tạo báo giá package bulk qua B2B portal (config layer — giao dịch tại M6) |

**Flow — M2-US-08/09**

```
 Product Manager                    Hệ thống (M2)                 B2B Customer
     │                                   │                              │
     │ 1. Bật/tắt hiển thị gói theo     │                              │
     │  từng channel; sort order riêng  │                              │
     │  theo channel; đánh dấu gói nổi  │                              │
     │  bật (ưu tiên trang chủ)         │                              │
     ├───────────────────────────────────▶                              │
     │                                   │◀─────────────────────────────┤
     │                                   │  2. Lọc gói theo sân bay,     │
     │                                   │  danh mục, mức chiết khấu     │
     │                                   │  theo volume                  │
     │                                   │  3. Tạo báo giá (quote) —     │
     │                                   │  chọn gói + số lượng          │
     │                                   │  4. Upload PO → M2 xác nhận   │
     │                                   │  cấu hình → chuyển M6 phát    │
     │                                   │  hành voucher (giao dịch      │
     │                                   │  thực tế nằm ngoài M2)        │
```

---

## 4. Luồng tổng thể xuyên module (End-to-end)

Cập nhật theo quyết định trong `ACE-ADR-0002-m1-m2.md` (`service.approved` là pull qua API, không
phải Kafka event — chỉ `price.changed`, `service.paused`/`deprecated` và `package.published` mới
cần outbox/Kafka).

```
 Nhà cung ứng                M1 (Supply)              M2 (Package)            ACE Admin
     │                            │                         │                     │
     │ 1. Tự đăng ký / ACE mời   │                         │                     │
     ├────────────────────────────▶                         │                     │
     │                            │  2. ACE Admin review KYC │                     │
     │                            ├─────────────────────────┼─────────────────────▶│
     │                            │◀──────────────────────────────────────────────┤
     │                            │  Supplier → active                            │
     │ 3. Khai báo dịch vụ        │                         │                     │
     ├────────────────────────────▶  status=draft            │                     │
     │                            │  → pending_review        │                     │
     │                            ├─────────────────────────┼─────────────────────▶│
     │                            │◀──────────────────────────────────────────────┤
     │                            │  4. Service → approved (snapshot v1, bất biến)│
     │                            │  KHÔNG phát Kafka event — sẵn sàng qua API     │
     │                            │                         │                     │
     │                            │◀────── 5. GET /v1/services/{id}/snapshots ────┤
     │                            │        (PM chủ động pull khi xây dựng gói)     │
     │                            │                         │                     │
     │                            │           6. PM xây dựng gói (M2-E1..E3):     │
     │                            │           chọn snapshot, targeting, giá,      │
     │                            │           revenue split, khuyến mại           │
     │                            │                         ├─────────────────────▶│
     │                            │                         │◀──────────────────────┤
     │                            │                         │  Package → active     │
     │                            │                         │  package_version tạo  │
     │                            │                         │  (bất biến, revenue    │
     │                            │                         │  split khoá)           │
     │                            │                         │  event: package.published
     │                            │                         │  → Kafka → M6 catalog  │
     │                            │                         │                     │
     ▼                            ▼                         ▼                     ▼
                        M6 Marketplace (downstream — ngoài phạm vi tài liệu này)
```

---

## 5. Cascade khi upstream thay đổi (bắt buộc Kafka)

Đây là nhóm sự kiện duy nhất trong M1/M2 thực sự cần outbox/Kafka, vì consumer phải tự phản ứng
mà không có ai chủ động hỏi (chi tiết cơ chế: `ACE-ADR-0002-m1-m2.md` §Thách thức 5, D5, D11):

```
  event: price.changed (M1 → Kafka)
  ──▶ M2 consumer: mọi package đang hiển thị bán có chứa entitlement liên quan
      → pending_review, tạm dừng bán ngay (hard stop — ADR-0002 D5)

  event: service.paused / service.deprecated (M1 → Kafka)
  ──▶ M2 consumer: package chứa snapshot liên quan → paused, cảnh báo PM (M1-US-07)

  event: package.published (M2 → Kafka)
  ──▶ M6 Marketplace consumer: catalog cập nhật version mới sẵn sàng bán
```

**Luồng khôi phục sau `price.changed`** — package không tự phục hồi, ACE Admin phải chủ động rà
soát (khác với PM tự sửa package đang active, vốn fork version draft chạy song song mà không làm
gián đoạn bán hàng — M2-US-04 AC3):

```
Package → pending_review (do price.changed — hard stop)
     │
     ▼
PM cập nhật margin/selling_price theo giá M1 mới (M2-US-05)
Finance Manager cấu hình lại revenue split cho version mới (M2-US-06 —
package_items của version mới, không kế thừa revenue_splits cũ)
     │
     ▼
ACE Admin review
     │
     ├── approve ──▶ package_version v+1 tạo (bất biến) — prices/
     │               package_items/revenue_splits mới. current_version_id
     │               → v+1, status = active. event: package.published →
     │               Kafka (package bán lại được trên M6)
     │
     └── từ chối / cần sửa thêm ──▶ status = draft (chỉ PM thấy); PM sửa
                     lại rồi submit lại → pending_review, lặp lại review
     │
     ▼
(7 ngày kể từ thời điểm thay đổi — M2-US-04 AC4) ACE Admin có thể rollback
current_version_id về version trước đó nếu version mới sai
```

Order đã đặt dưới version cũ (trước khi giá đổi) không bị ảnh hưởng trong suốt luồng này —
`order_items` đã khoá `package_version_id` + `price_snapshot` tại thời điểm `order.confirmed`.

---

## 6. Bảng truy vết (Traceability matrix)

| Epic | User Story | RFP §  | Sơ đồ luồng trong tài liệu này |
|------|-----------|--------|----------------------------------|
| M1-E1 | US-01, US-02, US-03 | 4.1.3.1 | §2.2 |
| M1-E2 | US-04, US-05, US-06, US-07 | 4.1.3.2 | §2.3 |
| M1-E3 | US-08, US-09, US-10 | 4.1.3.3 | §2.4 |
| M1-E4 | US-11, US-12, US-13 | 4.1.3.4 | §2.5 |
| M2-E1 | US-01, US-02, US-03, US-04 | 4.2.3.1 | §3.2 |
| M2-E2 | US-05, US-06, US-07 | 4.2.3.2 | §3.3 |
| M2-E3 | US-08, US-09 | 4.2.3.3 | §3.4 |
| — | — | — | End-to-end: §4, Cascade: §5 |

**22/22 use case** (M1: 13, M2: 9) từ RFP mục 4.1 và 4.2 đã được ánh xạ trong tài liệu này.
