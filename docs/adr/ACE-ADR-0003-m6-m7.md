# ADR-0003: Kiến trúc và Thiết kế Database M6 Digital Marketplace & Settlement Layer và M7 Partner, Contract & Commercial Management

---

## Bối cảnh

M6 và M7 tạo thành lớp giao dịch và lớp quản trị thương mại của nền tảng ACE. Nếu M1 chuẩn hoá nguồn cung và M2 đóng gói sản phẩm, thì M6 trả lời câu hỏi: **package đã duyệt biến thành doanh thu thực tế như thế nào** — từ browse, giỏ hàng, đặt hàng, thanh toán, phát hành voucher, redeem tại điểm dịch vụ, đến hoá đơn, đối soát và quyết toán. M7 trả lời câu hỏi khác: **điều khoản thương mại nào đang chi phối giao dịch đó** — đối tác nào được phép mua gì, giá hợp đồng và revenue split nào áp dụng, chu kỳ thanh toán ra sao, SLA cam kết thế nào, và bao nhiêu năng lực được giữ riêng cho đối tác nào.

M7 không nằm trong dòng chảy tuyến tính M1 → M2 → M6. Đây là lớp quản trị bao trùm, cung cấp dữ liệu điều khoản có cấu trúc để M2 (pricing) và M6 (checkout, billing, settlement) đọc và áp dụng tự động — không qua parse PDF thủ công.

M6 được tách thành **ba service độc lập** thay vì một service duy nhất, vì ba đặc tính latency/consistency khác nhau không thể dung hoà trong cùng một service:

- **M6 Marketplace** — giao dịch đồng bộ, cần phản hồi nhanh cho buyer (browse, cart, checkout).
- **M6 Voucher** — phải hoạt động cả khi kiosk mất mạng, xác thực offline dưới 1 giây.
- **M6 Settlement** — batch, yêu cầu chính xác tuyệt đối, không dung sai floating-point.

Sáu thách thức cốt lõi chi phối mọi quyết định kiến trúc và thiết kế database trong tài liệu này:

1. **Tính bất biến xuyên 4 module (Price Lock end-to-end)** — order, voucher, hoá đơn và settlement phát sinh tại M6 phải khoá đúng phiên bản của service snapshot (M1), package version (M2) và commercial term (M7) tại thời điểm giao dịch; thay đổi thượng nguồn sau đó không được hồi tố.
2. **Idempotency toàn tuyến cho thao tác tài chính** — checkout, phát hành voucher, redeem (kể cả ngoại tuyến), và quyết toán đều phải chống trùng tuyệt đối; một lần trùng là một lần sai số dư thực.
3. **Kiểm soát năng lực đa tầng** — capacity thực của NCC (M1) phải được chia theo cam kết thương mại của M7 (allotment/quota/commitment) và kiểm tra nhất quán tại nhiều bước của M6 mà không xung đột hay overbooking.
4. **Toàn vẹn tài chính đối soát → quyết toán** — ba luồng doanh thu (WS1 hoa hồng, WS2 bán sỉ, WS3 tự doanh) có công thức khác nhau; dữ liệu chưa đối soát tuyệt đối không được đưa vào quyết toán.
5. **Hợp đồng và điều khoản có cấu trúc với cơ chế resolve xung đột** — PDF chỉ là tham chiếu pháp lý; khi nhiều điều khoản cùng match một giao dịch, hệ thống phải resolve theo thứ tự xác định, không mơ hồ.
6. **Độ tin cậy khi ngoại tuyến và khi publish sự kiện** — kiosk redeem phải hoạt động khi mất mạng; cả bốn service (M6×3 + M7) phát Kafka event tại các state transition quan trọng phải đảm bảo không mất event và không làm lệch DB với broker.

---

## Phân tích thách thức

### Thách thức 1 — Tính bất biến xuyên 4 module (Price Lock end-to-end)

**Vấn đề:** Buyer xem giá package lúc browse; giữa lúc đó và lúc checkout, pricing rule có thể hết hiệu lực, service snapshot mới được publish, hoặc commercial term M7 thay đổi. Nếu order/voucher/invoice/settlement không khoá đúng "ảnh chụp" tại từng bước, tranh chấp giá về sau không có căn cứ giải quyết — buyer có thể khiếu nại giá đã trả không khớp giá công bố, còn Finance không thể chứng minh giá nào đã áp dụng tại thời điểm nào.

**Giải pháp — Snapshot chain nối dài từ M1/M2 sang M6/M7:**

```
  service_snapshot (M1, bất biến)
        │
        ▼
  package_version (M2, bất biến)
        │
        ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  cart_items.unit_price_locked                               │
  │  • khoá 15 phút kể từ add-to-cart (price_lock_expires_at)   │
  │  • hết hạn → phải re-confirm giá trước checkout             │
  └────────────────────────────┬────────────────────────────────┘
                               │
  ┌─────────────────────────────────────────────────────────────┐
  │  quotes.snapshot_data (JSONB)                               │
  │  • đóng băng package, số lượng, giá, buyer entity, điều     │
  │    khoản thương mại tại thời điểm tạo quote                 │
  │  • hiệu lực 48 giờ; hết hạn KHÔNG được dùng lại tạo order   │
  └────────────────────────────┬────────────────────────────────┘
                               │
  ┌─────────────────────────────────────────────────────────────┐
  │  order_items.package_version_id + price_snapshot            │
  │  • khoá tại order.confirmed — không phải package_id sống    │
  │  • commercial_term_version_id + revenue_split_version_id    │
  │    khoá cùng thời điểm (nguồn cho settlement sau này)       │
  └────────────────────────────┬────────────────────────────────┘
                               │
  ┌─────────────────────────────────────────────────────────────┐
  │  vouchers.snapshot_data (JSONB)                             │
  │  • giá, entitlement, airport, timeslot, điều kiện sử dụng,  │
  │    redemption_limit — đóng băng tại thời điểm issue         │
  └────────────────────────────┬────────────────────────────────┘
                               │
  ┌─────────────────────────────────────────────────────────────┐
  │  settlements.commercial_term_version_id                     │
  │           .revenue_split_version_id                         │
  │  • dùng đúng version đã khoá tại order.confirmed, KHÔNG     │
  │    dùng version hiện hành của hợp đồng tại lúc quyết toán   │
  └─────────────────────────────────────────────────────────────┘
```

Quyết định: **D2**, **D3**, **D4**, **D9**

---

### Thách thức 2 — Idempotency toàn tuyến cho thao tác tài chính

**Vấn đề:** Gateway timeout khiến client retry checkout; callback thanh toán gọi lặp; kiosk sync lại dữ liệu ngoại tuyến sau khi mất mạng; settlement job retry sau lỗi một phần — mỗi điểm này đều có thể tạo bản ghi tài chính trùng nếu không có cơ chế chống trùng tường minh.

**Giải pháp — Idempotency-Key bắt buộc tại mọi API mutation + composite unique key ở DB:**

```
  Tầng gateway:  Idempotency-Key header bắt buộc cho mọi API mutation
                 → thiếu header → HTTP 400
                 Valkey SETNX dedup, TTL 24h

  Tầng DB:       orders.idempotency_key                    UNIQUE
                 vouchers.issuance_idempotency_key         UNIQUE
                 voucher_redemptions.idempotency_key        UNIQUE
                 settlements: UNIQUE (supplier_id, period, voucher_id)
                              + idempotency_key

  Bulk voucher:  lỗi một phần → chỉ retry voucher lỗi, giữ nguyên
                 voucher đã issue thành công (processing_status
                 per-item: pending/success/failed)
```

Quyết định: **D3**, **D5**, **D8**

---

### Thách thức 3 — Kiểm soát năng lực đa tầng (Inventory + Allotment/Quota/Commitment)

**Vấn đề:** Capacity thực tại M1 (ví dụ lounge có 100 ghế) phải được chia sẻ giữa nhiều đối tác B2B theo cam kết hợp đồng khác nhau. Nếu M6 chỉ kiểm tra `capacity_total - capacity_used` mà không biết phần nào đã dành riêng cho đối tác nào, một partner có thể vô tình (hoặc qua nhu cầu đột biến) mua hết phần capacity đáng lẽ dành cho partner khác — vi phạm cam kết hợp đồng dù tổng capacity vẫn còn.

**Giải pháp — Allotment là lớp overlay có version trên capacity M1, kiểm tra tại 4 checkpoint:**

```
  M1 inventory.capacity_total = 100 (lounge, 1 ngày)
        │
        ▼
  M7 allotments (overlay có version, scope: partner × airport × service × channel × thời gian)
  ┌─────────────────┬──────────────────┬───────────┬──────────────────┐
  │  HDBank         │  VietJet         │  Buffer   │  Free-sell B2C   │
  │  30 ghế         │  20 ghế          │  10 ghế   │  40 ghế          │
  └─────────────────┴──────────────────┴───────────┴──────────────────┘
        │
        ▼ Kiểm tra tại 4 điểm trong M6:
   1. Browse (M6.4.1)        — chỉ hiển thị nếu partner còn allotment/quota
   2. Add-to-cart (soft)     — cảnh báo nếu gần hết buffer, KHÔNG chặn cứng
   3. Checkout (hard)        — kiểm tra lại allotment còn khả dụng trước confirm
   4. Voucher issuance       — KHÔNG phát hành vượt số đã phân bổ/cam kết
```

Vượt allotment vẫn cho giao dịch nếu policy cho phép, nhưng phải hiển thị `over_allotment_price` và cảnh báo Partner Manager — không âm thầm chặn cũng không âm thầm cho qua.

Quyết định: **D6**, **D11**

---

### Thách thức 4 — Toàn vẹn tài chính đối soát → quyết toán

**Vấn đề:** Ba mô hình kinh doanh (WS1 bán hộ hưởng hoa hồng, WS2 bán buôn B2B2C, WS3 tự doanh) có công thức tính net payable khác nhau. Dữ liệu redeem có ba nguồn (ACE, NCC, kiosk) có thể lệch nhau về số lượng hoặc giá tại thời điểm tiêu thụ. Nếu quyết toán chạy trên dữ liệu chưa đối soát, ACE trả sai tiền cho NCC và không thể giải trình khi có tranh chấp.

**Giải pháp — Six-state reconciliation gate trước khi vào settlement + append-only ledger double-entry + Two-Ring Settlement:**

```
  Pending Data ──▶ Matched ──────────────────────▶ (đủ điều kiện)
       │              │
       │         Mismatch ──▶ Pending Review ──▶ Adjusted
       │                                              │
       └── (chưa đủ dữ liệu, không mất — chờ kỳ sau)   ▼
                                                   Reconciled
                                                       │
                        chỉ Reconciled / Adjusted&Approved đi tiếp
                                                       ▼
                                        ┌──────────────────────────┐
                                        │  Settlement (append-only)│
                                        │  Ring 1: ACE ↔ NCC (M1)  │
                                        │  Ring 2: ACE ↔ Partner   │
                                        │          B2B (M7)        │
                                        └──────────────────────────┘

  |B (ACE ghi nhận) − C (NCC báo cáo)| > 0  ──▶  tự động block payout,
                                                  mở ticket, không tự
                                                  ghi đè dữ liệu gốc
```

Quyết định: **D7**, **D8**, **D10**

---

### Thách thức 5 — Hợp đồng và điều khoản có cấu trúc với cơ chế resolve xung đột

**Vấn đề:** File PDF hợp đồng không thể là input cho engine tính giá, kiểm tra checkout, xuất hoá đơn hay quyết toán. Đồng thời, nhiều điều khoản có thể cùng match một giao dịch — ví dụ điều khoản chung cho mọi sân bay và điều khoản riêng cho HDBank tại Long Thành — nếu không có rule resolve rõ ràng, hệ thống có thể áp sai giá, sai billing model hoặc cho phép channel visibility sai.

**Giải pháp — Two-layer contract data + resolve order xác định:**

```
  Contract (mutable metadata: tên, partner, loại HĐ, hiệu lực)
        │  ACE phê duyệt → Active
        ▼
  Commercial Terms (structured, versioned — KHÔNG sửa trực tiếp khi Active)
        │  mỗi thay đổi → amendment/version mới có effective_from riêng
        ▼
  Khi giao dịch phát sinh ở M2/M6, resolve theo thứ tự:
  ┌─────────────────────────────────────────────────────────────┐
  │  1. Phạm vi cụ thể hơn thắng phạm vi chung                  │
  │     (HDBank @ Long Thành thắng "mọi sân bay")               │
  │  2. Nếu cùng độ cụ thể, priority cao hơn thắng              │
  │  3. Nếu xung đột allow/deny, deny thắng allow               │
  │  4. Nếu còn nhiều version hợp lệ, version mới nhất có hiệu  │
  │     lực tại thời điểm giao dịch thắng                       │
  │  Không resolve được → raise alert cho Partner Manager       │
  └─────────────────────────────────────────────────────────────┘
```

PDF vẫn được lưu làm tài liệu tham chiếu pháp lý, nhưng khi metadata hệ thống khác nội dung PDF, hệ thống chỉ cảnh báo — không tự động thay thế dữ liệu vận hành.

Quyết định: **D9**, **D10**, **D13**

---

### Thách thức 6 — Độ tin cậy khi ngoại tuyến và khi publish sự kiện

**Vấn đề:** Kiosk tại sân bay có thể mất kết nối mạng bất kỳ lúc nào; nếu redeem chặn cứng theo kết nối, ACE vi phạm cam kết QR ≤ 1 giây và làm gián đoạn trải nghiệm hành khách. Song song đó, cả bốn service (M6 Marketplace, M6 Voucher, M6 Settlement, M7 Partner) đều phát Kafka event tại các state transition quan trọng (`order.confirmed`, `voucher.redeemed`, `settlement.calculated`, `contract.activated`) — publish sau khi commit DB tạo khoảng thời gian crash có thể làm mất event vĩnh viễn, giống rủi ro đã phân tích ở ADR-0002.

**Giải pháp — Offline-first JWT verification + SQLite local cache tại kiosk + Transactional Outbox Pattern tái sử dụng trên cả 4 service:**

```
  Kiosk (mất kết nối):
  ┌─────────────────────────────────────────────────────────────┐
  │  Xác thực JWT cục bộ bằng public key RS256 đã cache (24h)   │
  │  Kiểm tra trùng qua SQLite cục bộ trên thiết bị             │
  │  Ghi nhận redeem vào hàng đợi cục bộ                        │
  │  Khi có mạng lại: sync ngược lên ACE với idempotency_key    │
  │  Độ trễ đồng bộ ≤ 5 giây; sai lệch tồn kho tối đa ≤ 0,1%    │
  └─────────────────────────────────────────────────────────────┘

  Mỗi service (M6×3 + M7), trong cùng 1 DB transaction:
  ┌─────────────────────────────────────────────────────────────┐
  │  BEGIN                                                      │
  │  UPDATE <entity> SET status = '<new_state>'                 │
  │  INSERT INTO outbox (event_type, payload)                   │
  │  COMMIT                                                     │
  └────────────────────────────┬────────────────────────────────┘
                               │
  Outbox Relay (SELECT ... FOR UPDATE SKIP LOCKED) → Kafka → published
  Consumer idempotent qua processed_events (ON CONFLICT DO NOTHING)
```

Quyết định: **D5**, **D12**

---

## Luồng hệ thống tổng thể

```
 ┌────────────────────────────────────────────────────────────────────────────┐
 │                    M6 + M7 — LUỒNG NGHIỆP VỤ TỔNG THỂ                      │
 └────────────────────────────────────────────────────────────────────────────┘

  ┌──────────────┐
  │  M2 Package  │  package_version_id sẵn sàng (từ ADR-0002)
  └──────┬───────┘
         │
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M7: Partner đã Active, Contract đã Active                   │
  │  Commercial Terms resolve: scope, giá, billing model, SLA,   │
  │  allotment/quota — cung cấp cho M2 (đã dùng ở ADR-0002)      │
  │  và M6 (dùng ở luồng dưới đây)                               │
  └──────┬───────────────────────────────────────────────────────┘
         │ 1. B2B Customer browse Marketplace
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M6 Marketplace: hiển thị package theo channel visibility    │
  │  (M7 scope) + còn allotment/quota + entitlement còn bán      │
  │  Add-to-cart: khoá giá 15 phút (cart_items)                  │
  └──────┬───────────────────────────────────────────────────────┘
         │ 2a. luồng Quote → Order              │ 2b. luồng trực tiếp
         ▼                                       ▼
  ┌───────────────────────────┐         ┌───────────────────────────┐
  │  Quote (snapshot 48h)     │         │  Checkout trực tiếp       │
  │  Upload PO → ACE duyệt    │         │  Kiểm tra cứng: giá,      │
  └──────────┬────────────────┘         │  capacity, allotment,     │
             │                          │  commercial terms         │
             └──────────┬───────────────┴───────────────────────────┘
                        ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  Order confirmed (idempotency_key)                           │
  │  order_items khoá package_version_id + price_snapshot +      │
  │  commercial_term_version_id + revenue_split_version_id       │
  │  event: order.confirmed → Kafka                              │
  │  ──▶ M1: reservation giữ chỗ inventory (giai đoạn 1)         │
  └──────┬───────────────────────────────────────────────────────┘
         │ 3. thanh toán đủ điều kiện theo payment mode
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M6 Voucher: phát hành voucher (snapshot bất biến, JWT ký)   │
  │  mỗi entitlement unit → 1 voucher; idempotency per issue     │
  │  event: voucher.issued → Kafka                               │
  └──────┬───────────────────────────────────────────────────────┘
         │ 4. redeem tại điểm dịch vụ (online hoặc offline)
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M6 Voucher: redeem — xác thực JWT, kiểm tra trùng,          │
  │  kiểm tra airport/timeslot/scope                             │
  │  ──▶ M1: reservation xác nhận (giai đoạn 2 — tiêu thụ thực)  │
  │  event: voucher.redeemed → Kafka                             │
  └──────┬───────────────────────────────────────────────────────┘
         │ 5. chu kỳ đối soát T+n (per supplier, mặc định T+1)
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M6 Settlement: Reconciliation — so khớp ACE/NCC/kiosk       │
  │  → Matched/Mismatch/Adjusted/Reconciled                      │
  └──────┬───────────────────────────────────────────────────────┘
         │ 6. đủ 6 điều kiện quyết toán
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M6 Settlement: Settlement (append-only)                     │
  │  áp commercial_term_version_id + revenue_split_version_id    │
  │  đã khoá tại order.confirmed — KHÔNG dùng version hiện hành  │
  │  Ring 1: ACE ↔ NCC (M1) | Ring 2: ACE ↔ B2B Partner (M7)     │
  │  Finance duyệt → payment instruction (Phase 1: report-only)  │
  └──────┬───────────────────────────────────────────────────────┘
         │ song song
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M6 Settlement: Invoice — theo 1 trong 5 mô hình (§M6.6.1)   │
  │  ưu tiên theo package để bảo vệ Price Opacity                │
  └──────────────────────────────────────────────────────────────┘


 ┌────────────────────────────────────────────────────────────────────────────┐
 │                     CASCADE KHI M1/M7 THAY ĐỔI                             │
 └────────────────────────────────────────────────────────────────────────────┘

  event: service.paused/deprecated (M1, Kafka)
  ──▶ M2 consumer: package → paused (đã xử lý ở ADR-0002)
  ──▶ M6 Voucher consumer: voucher liên quan → UNAVAILABLE, auto-refund

  event: contract.expiring / contract.terminated (M7, Kafka)
  ──▶ M6 Marketplace consumer: chặn order mới cho partner liên quan
  ──▶ voucher đã phát hành trước đó: xử lý theo policy hợp đồng (KHÔNG
      tự huỷ trừ vi phạm nghiêm trọng có quyết định riêng)

  event: commercial_term.amended (M7, Kafka — version mới)
  ──▶ M2 consumer: package dùng term liên quan → pending_review
  ──▶ M6 Settlement: giao dịch MỚI dùng version mới; giao dịch cũ giữ
      nguyên version đã khoá tại order.confirmed (Price Lock)

  event: partner.suspended (M7, Kafka)
  ──▶ M6 Marketplace consumer: chặn order mới theo phạm vi bị ảnh hưởng
  ──▶ voucher cũ vẫn redeem được nếu policy hợp đồng cho phép


 ┌────────────────────────────────────────────────────────────────────────────┐
 │                              ERD                                           │
 └────────────────────────────────────────────────────────────────────────────┘

  M6 Marketplace (schema: m6_marketplace — Aurora instance riêng)
  b2b_customers ──< b2b_customer_users
  b2b_customers ──< carts ──< cart_items ──▶ UUID package_version_id (M2)
                ──< quotes ──< quote_items
                              └──< purchase_orders
                ──< orders ──< order_items ──▶ UUID package_version_id (M2)
                                            ──▶ UUID commercial_term_id (M7)
                          ──< transactions
  outbox (relay → Kafka)

  M6 Voucher (schema: m6_voucher — Aurora instance riêng)
  consumers ──< vouchers ──▶ UUID order_item_id (M6 Marketplace)
                          ──▶ UUID snapshot_id (M1 service_snapshots)
                          ──▶ UUID package_version_id (M2)
            vouchers ──< voucher_redemptions ──▶ kiosks
  outbox (relay → Kafka)

  M6 Settlement (schema: m6_settlement — Aurora instance riêng)
  billing_cycles ──< invoices ──< invoice_lines
                              ──< invoice_adjustments
  disputes ──< refund_requests
  reconciliation_batches ──< reconciliation_items ──▶ UUID voucher_id (M6 Voucher)
  settlements (append-only) ──< settlement_lines
                            ──▶ UUID commercial_term_version_id (M7)
                            ──▶ UUID revenue_split_version_id (M2)
  ledger_entries (append-only, double-entry)
  fraud_rules ──< fraud_rule_versions (immutable)
  outbox (relay → Kafka)

  M7 Partner (schema: m7_partner — Aurora instance riêng)
  partners ──< partner_users
  partners ──▶ UUID linked_supplier_id (M1, quan hệ hai chiều)
  partners ──< contracts ──< commercial_terms (versioned)
                                        ──< sla_configs ──< sla_metrics
           ──< scorecards
           ──< allotments
           ──< quotas
           ──< commitments
           ──< communication_threads ──< communication_messages
  outbox (relay → Kafka)
```

---

## DDL

### M6 Marketplace — Schema `m6_marketplace`

```sql
CREATE SCHEMA m6_marketplace;

-- ----------------------------------------------------------------
-- B2B CUSTOMERS
-- partner_id tham chiếu m7_partner.partners (UUID, không FK).
-- credit_used/credit_limit hỗ trợ banner cảnh báo 80/95/100%.
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.b2b_customers (
    customer_id       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id        UUID          NOT NULL,
    tax_code          VARCHAR(20)   NOT NULL UNIQUE,
    company_name      VARCHAR(255)  NOT NULL,
    billing_model     VARCHAR(20)   NOT NULL DEFAULT 'prepaid'
                          CHECK (billing_model IN ('prepaid','credit_line','postpaid_invoice')),
    credit_limit      NUMERIC(15,2) NOT NULL DEFAULT 0,
    credit_used       NUMERIC(15,2) NOT NULL DEFAULT 0,
    overdue_days      INT           NOT NULL DEFAULT 0,
    status            VARCHAR(20)   NOT NULL DEFAULT 'active'
                          CHECK (status IN ('pending','active','credit_hold','suspended')),
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),

    CONSTRAINT credit_used_non_negative CHECK (credit_used >= 0)
);

CREATE INDEX ON m6_marketplace.b2b_customers (partner_id);
CREATE INDEX ON m6_marketplace.b2b_customers (status);

-- ----------------------------------------------------------------
-- B2B CUSTOMER USERS
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.b2b_customer_users (
    user_id     UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID         NOT NULL REFERENCES m6_marketplace.b2b_customers,
    email       VARCHAR(255) NOT NULL,
    role        VARCHAR(20)  NOT NULL CHECK (role IN ('admin','buyer','viewer')),
    status      VARCHAR(20)  NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.b2b_customer_users (customer_id);

-- ----------------------------------------------------------------
-- CARTS
-- Mỗi customer chỉ có một cart 'open' tại một thời điểm.
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.carts (
    cart_id     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID        NOT NULL REFERENCES m6_marketplace.b2b_customers,
    user_id     UUID        NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open','converted','abandoned')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ON m6_marketplace.carts (customer_id) WHERE status = 'open';

-- ----------------------------------------------------------------
-- CART ITEMS
-- Giá khoá 15 phút kể từ add-to-cart (M6.A1); package_version_id
-- tham chiếu m2_package.package_versions (UUID, không FK).
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.cart_items (
    cart_item_id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id                UUID          NOT NULL REFERENCES m6_marketplace.carts,
    package_version_id     UUID          NOT NULL,
    quantity               INT           NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_price_locked      NUMERIC(15,2) NOT NULL,
    timeslot_id            UUID,
    price_locked_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    price_lock_expires_at  TIMESTAMPTZ   NOT NULL DEFAULT (now() + interval '15 minutes'),
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.cart_items (cart_id);
CREATE INDEX ON m6_marketplace.cart_items (price_lock_expires_at);

-- ----------------------------------------------------------------
-- QUOTES (snapshot 48h — M6.4.2)
-- Hết hạn hoặc bị huỷ: KHÔNG được dùng lại để tạo order.
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.quotes (
    quote_id      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id   UUID        NOT NULL REFERENCES m6_marketplace.b2b_customers,
    cart_id       UUID        REFERENCES m6_marketplace.carts,
    snapshot_data JSONB       NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','converted','expired','cancelled')),
    valid_until   TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '48 hours'),
    created_by    UUID        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.quotes (customer_id, status);
CREATE INDEX ON m6_marketplace.quotes (valid_until) WHERE status = 'active';

-- ----------------------------------------------------------------
-- QUOTE ITEMS
-- tier_price khoá giá B2B tier tại thời điểm báo giá.
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.quote_items (
    quote_item_id      UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id           UUID          NOT NULL REFERENCES m6_marketplace.quotes,
    package_version_id UUID          NOT NULL,
    quantity           INT           NOT NULL DEFAULT 1 CHECK (quantity > 0),
    tier_price         NUMERIC(15,2) NOT NULL,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.quote_items (quote_id);

-- ----------------------------------------------------------------
-- PURCHASE ORDERS (PO upload gắn với quote)
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.purchase_orders (
    po_id       UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    po_number   VARCHAR(100) NOT NULL UNIQUE,
    quote_id    UUID         NOT NULL REFERENCES m6_marketplace.quotes,
    file_url    TEXT         NOT NULL,
    status      VARCHAR(20)  NOT NULL DEFAULT 'submitted'
                    CHECK (status IN ('submitted','approved','rejected')),
    reviewed_by UUID,
    uploaded_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.purchase_orders (quote_id);

-- ----------------------------------------------------------------
-- ORDERS
-- idempotency_key chống trùng khi gateway timeout + client retry
-- (RFP M6-US-05 AC10 / CP-1).
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.orders (
    order_id         UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key  VARCHAR(100)  NOT NULL UNIQUE,
    customer_id      UUID          NOT NULL REFERENCES m6_marketplace.b2b_customers,
    quote_id         UUID          REFERENCES m6_marketplace.quotes,
    source           VARCHAR(10)   NOT NULL CHECK (source IN ('cart','quote')),
    payment_method_id UUID,
    status           VARCHAR(20)   NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','confirmed','cancelled','closed')),
    payment_status   VARCHAR(20)   NOT NULL DEFAULT 'pending_payment'
                          CHECK (payment_status IN (
                              'pending_payment','paid','payment_failed',
                              'credit_approved','invoice_pending'
                          )),
    total_amount     NUMERIC(15,2) NOT NULL,
    currency         VARCHAR(3)    NOT NULL DEFAULT 'VND',
    confirmed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.orders (customer_id, status);
CREATE INDEX ON m6_marketplace.orders (payment_status);

-- ----------------------------------------------------------------
-- ORDER ITEMS
-- Khoá package_version_id + price_snapshot tại order.confirmed.
-- commercial_term_version_id/revenue_split_version_id khoá cùng lúc
-- để settlement dùng đúng điều khoản đã áp tại giao dịch (D9).
-- allotment_id (M7) nullable — chỉ set nếu tiêu thụ từ allotment
-- riêng của partner thay vì free-sell.
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.order_items (
    order_item_id             UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id                  UUID          NOT NULL REFERENCES m6_marketplace.orders,
    package_version_id        UUID          NOT NULL,
    quantity                  INT           NOT NULL DEFAULT 1 CHECK (quantity > 0),
    price_snapshot            NUMERIC(15,2) NOT NULL,
    commercial_term_version_id UUID         NOT NULL,
    revenue_split_version_id  UUID          NOT NULL,
    allotment_id              UUID,
    over_allotment_qty        INT           NOT NULL DEFAULT 0,
    created_at                TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.order_items (order_id);
CREATE INDEX ON m6_marketplace.order_items (package_version_id);

-- ----------------------------------------------------------------
-- PAYMENT METHODS
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.payment_methods (
    method_id    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id  UUID        NOT NULL REFERENCES m6_marketplace.b2b_customers,
    type         VARCHAR(20) NOT NULL
                     CHECK (type IN ('card','ewallet','bank_transfer','b2b_credit')),
    provider_ref VARCHAR(100),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.payment_methods (customer_id);

-- ----------------------------------------------------------------
-- TRANSACTIONS
-- gateway_txn_id phục vụ đối soát T+n với payment gateway.
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.transactions (
    txn_id         UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID          NOT NULL REFERENCES m6_marketplace.orders,
    gateway        VARCHAR(30)   NOT NULL,
    gateway_txn_id VARCHAR(100),
    amount         NUMERIC(15,2) NOT NULL,
    status         VARCHAR(20)   NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','success','failed','refunded')),
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_marketplace.transactions (order_id);
CREATE INDEX ON m6_marketplace.transactions (gateway_txn_id);

-- ----------------------------------------------------------------
-- OUTBOX (Transactional Outbox Pattern — tái sử dụng D11 của ADR-0002)
-- ----------------------------------------------------------------
CREATE TABLE m6_marketplace.outbox (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(50)  NOT NULL,
    aggregate_id   UUID         NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB        NOT NULL,
    status         VARCHAR(20)  NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','published','failed')),
    retry_count    INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX ON m6_marketplace.outbox (status, created_at) WHERE status = 'pending';
```

> M6 Voucher, M6 Settlement và M7 đều có bảng `outbox` tương tự — cấu trúc giống hệt, không lặp lại DDL ở các schema dưới đây.

### M6 Voucher — Schema `m6_voucher`

```sql
CREATE SCHEMA m6_voucher;

-- ----------------------------------------------------------------
-- CONSUMERS
-- ace_uid ẩn danh, KHÔNG chứa PII (M6.A5). opid_ref nullable —
-- để trống sẵn cho Phase 2, cùng pattern opid_ref của M1 (ADR-0002).
-- ----------------------------------------------------------------
CREATE TABLE m6_voucher.consumers (
    consumer_id     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    ace_uid         VARCHAR(64) NOT NULL UNIQUE,
    opid_ref        VARCHAR(100),
    b2b_customer_id UUID        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_voucher.consumers (b2b_customer_id);

-- ----------------------------------------------------------------
-- VOUCHERS
-- snapshot_data đóng băng tại thời điểm issue (D4). snapshot_id
-- và package_version_id tham chiếu M1/M2 (UUID, không FK).
-- Bảy trạng thái theo state machine ADR kỹ thuật §10.1.
-- ----------------------------------------------------------------
CREATE TABLE m6_voucher.vouchers (
    voucher_id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    qr_token                  VARCHAR(500)  NOT NULL UNIQUE,
    order_item_id             UUID          NOT NULL,
    snapshot_id               UUID          NOT NULL,
    package_version_id        UUID          NOT NULL,
    consumer_id               UUID          NOT NULL REFERENCES m6_voucher.consumers,
    entitlement_unit          INT           NOT NULL DEFAULT 1,
    issuance_idempotency_key  VARCHAR(100)  NOT NULL UNIQUE,
    status                    VARCHAR(20)   NOT NULL DEFAULT 'issued'
                                  CHECK (status IN (
                                      'issued','allocated','redeemed','cancelled',
                                      'expired','unavailable','settled'
                                  )),
    snapshot_data             JSONB         NOT NULL,
    valid_from                TIMESTAMPTZ   NOT NULL,
    valid_until                TIMESTAMPTZ   NOT NULL,
    partner_delivery_status   VARCHAR(20)   NOT NULL DEFAULT 'pending'
                                  CHECK (partner_delivery_status IN ('pending','delivered','failed')),
    issued_at                 TIMESTAMPTZ   NOT NULL DEFAULT now(),
    created_at                TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_voucher.vouchers (order_item_id);
CREATE INDEX ON m6_voucher.vouchers (consumer_id, status);
CREATE INDEX ON m6_voucher.vouchers (valid_until) WHERE status IN ('issued','allocated');

-- ----------------------------------------------------------------
-- KIOSKS
-- device_serial là khoá duy nhất để truy vết nguồn gốc redemption.
-- ----------------------------------------------------------------
CREATE TABLE m6_voucher.kiosks (
    kiosk_id       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    device_serial  VARCHAR(100) NOT NULL UNIQUE,
    supplier_id    UUID        NOT NULL,
    airport_code   VARCHAR(10) NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
    last_synced_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_voucher.kiosks (supplier_id, airport_code);

-- ----------------------------------------------------------------
-- VOUCHER REDEMPTIONS
-- idempotency_key chống double-redeem khi mạng chập chờn hoặc khi
-- sync ngoại tuyến (D5). offline_synced/sync_delay_ms phục vụ theo
-- dõi cam kết đồng bộ ≤5s sau khi kiosk có mạng trở lại.
-- ----------------------------------------------------------------
CREATE TABLE m6_voucher.voucher_redemptions (
    redemption_id   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    voucher_id      UUID        NOT NULL REFERENCES m6_voucher.vouchers,
    kiosk_id        UUID        NOT NULL REFERENCES m6_voucher.kiosks,
    idempotency_key VARCHAR(100) NOT NULL UNIQUE,
    airport_code    VARCHAR(10) NOT NULL,
    service_point   VARCHAR(100),
    usage_count     INT         NOT NULL DEFAULT 1,
    offline_synced  BOOLEAN     NOT NULL DEFAULT false,
    sync_delay_ms   INT,
    redeemed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_voucher.voucher_redemptions (voucher_id);
CREATE INDEX ON m6_voucher.voucher_redemptions (kiosk_id, redeemed_at);

CREATE TABLE m6_voucher.outbox (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(50)  NOT NULL,
    aggregate_id   UUID         NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB        NOT NULL,
    status         VARCHAR(20)  NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','published','failed')),
    retry_count    INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX ON m6_voucher.outbox (status, created_at) WHERE status = 'pending';
```

### M6 Settlement — Schema `m6_settlement`

```sql
CREATE SCHEMA m6_settlement;

-- ----------------------------------------------------------------
-- BILLING CYCLES
-- Gom nhiều order trong kỳ thành một invoice (mô hình billing cycle).
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.billing_cycles (
    billing_cycle_id UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id      UUID        NOT NULL,
    cycle_start      DATE        NOT NULL,
    cycle_end        DATE        NOT NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'open'
                         CHECK (status IN ('open','closed','invoiced')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.billing_cycles (customer_id, status);

-- ----------------------------------------------------------------
-- INVOICES
-- invoice_model theo 1 trong 5 mô hình phát hành hoá đơn (M6.6.1).
-- invoice_number UNIQUE theo yêu cầu cơ quan thuế (NĐ 123/2020).
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.invoices (
    invoice_id       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_number   VARCHAR(50)   NOT NULL UNIQUE,
    customer_id      UUID          NOT NULL,
    billing_cycle_id UUID          REFERENCES m6_settlement.billing_cycles,
    invoice_model    VARCHAR(20)   NOT NULL
                          CHECK (invoice_model IN (
                              'immediate','billing_cycle','per_package',
                              'per_entitlement','usage_based'
                          )),
    amount           NUMERIC(15,2) NOT NULL,
    vat_amount       NUMERIC(15,2) NOT NULL DEFAULT 0,
    currency         VARCHAR(3)    NOT NULL DEFAULT 'VND',
    status           VARCHAR(20)   NOT NULL DEFAULT 'draft'
                          CHECK (status IN ('draft','issued','adjusted','void')),
    issued_at        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.invoices (customer_id, status);
CREATE INDEX ON m6_settlement.invoices (billing_cycle_id);

-- ----------------------------------------------------------------
-- INVOICE LINES
-- Ưu tiên xuất theo package hơn theo entitlement để bảo vệ Price
-- Opacity (không lộ giá vốn/margin thành phần).
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.invoice_lines (
    invoice_line_id UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id      UUID          NOT NULL REFERENCES m6_settlement.invoices,
    order_item_id   UUID          NOT NULL,
    description     TEXT          NOT NULL,
    amount          NUMERIC(15,2) NOT NULL,
    vat_rate        NUMERIC(5,2)  NOT NULL DEFAULT 10.0,
    vat_amount      NUMERIC(15,2) NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.invoice_lines (invoice_id);

-- ----------------------------------------------------------------
-- INVOICE ADJUSTMENTS (append-only)
-- Một hoá đơn điều chỉnh có thể tham chiếu nhiều hoá đơn gốc; mỗi
-- dòng phải tham chiếu rõ hoá đơn gốc (M6.6.2). allocation_ratio
-- snapshot tại thời điểm order.confirmed.
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.invoice_adjustments (
    adjustment_id       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    original_invoice_id UUID          NOT NULL REFERENCES m6_settlement.invoices,
    adjustment_invoice_id UUID        NOT NULL REFERENCES m6_settlement.invoices,
    reason              TEXT          NOT NULL,
    amount              NUMERIC(15,2) NOT NULL,
    allocation_ratio     NUMERIC(5,4),
    created_by          UUID          NOT NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.invoice_adjustments (original_invoice_id);

-- ----------------------------------------------------------------
-- DISPUTES
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.disputes (
    dispute_id  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID        NOT NULL,
    voucher_id  UUID,
    dispute_type VARCHAR(30) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open','under_review','resolved','rejected')),
    opened_by   UUID        NOT NULL,
    resolved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.disputes (order_id);
CREATE INDEX ON m6_settlement.disputes (status);

-- ----------------------------------------------------------------
-- REFUND REQUESTS
-- Refund liên quan dispute → giữ chờ đến khi dispute xử lý xong.
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.refund_requests (
    refund_id     UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_item_id UUID          NOT NULL,
    voucher_id    UUID,
    dispute_id    UUID          REFERENCES m6_settlement.disputes,
    reason        TEXT          NOT NULL,
    refund_ratio  NUMERIC(5,4)  NOT NULL DEFAULT 1.0,
    amount        NUMERIC(15,2) NOT NULL,
    status        VARCHAR(20)   NOT NULL DEFAULT 'requested'
                      CHECK (status IN ('requested','approved','rejected','refunded')),
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.refund_requests (order_item_id);
CREATE INDEX ON m6_settlement.refund_requests (status);

-- ----------------------------------------------------------------
-- RECONCILIATION BATCHES
-- Sáu trạng thái đối soát (M6.7.3); nguồn dữ liệu theo M6.7.1.
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.reconciliation_batches (
    batch_id     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id  UUID        NOT NULL,
    period_start DATE        NOT NULL,
    period_end   DATE        NOT NULL,
    source       VARCHAR(20) NOT NULL
                     CHECK (source IN (
                         'ace_platform','supplier_api','kiosk','file_upload','adjustment'
                     )),
    status       VARCHAR(20) NOT NULL DEFAULT 'pending_data'
                     CHECK (status IN (
                         'pending_data','matched','mismatch',
                         'pending_review','adjusted','reconciled'
                     )),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.reconciliation_batches (supplier_id, status);

-- ----------------------------------------------------------------
-- RECONCILIATION ITEMS
-- ace_amount (B) vs supplier_amount (C); variance = B - C.
-- |variance| > 0 → block payout tự động, mở ticket (M6.7.6).
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.reconciliation_items (
    item_id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id           UUID          NOT NULL REFERENCES m6_settlement.reconciliation_batches,
    voucher_id         UUID          NOT NULL,
    ace_amount         NUMERIC(15,2) NOT NULL,
    supplier_amount    NUMERIC(15,2),
    variance_amount    NUMERIC(15,2) GENERATED ALWAYS AS (ace_amount - COALESCE(supplier_amount, ace_amount)) STORED,
    status             VARCHAR(20)   NOT NULL DEFAULT 'pending_data'
                            CHECK (status IN (
                                'pending_data','matched','mismatch',
                                'pending_review','adjusted','reconciled'
                            )),
    resolution_note    TEXT,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.reconciliation_items (batch_id, status);
CREATE INDEX ON m6_settlement.reconciliation_items (voucher_id);
CREATE INDEX ON m6_settlement.reconciliation_items (variance_amount) WHERE variance_amount <> 0;

-- ----------------------------------------------------------------
-- SETTLEMENTS (append-only)
-- Chỉ record Reconciled/Adjusted&Approved mới vào đây (D7).
-- commercial_term_version_id/revenue_split_version_id dùng đúng
-- version đã khoá tại order.confirmed — KHÔNG dùng version hiện
-- hành tại thời điểm quyết toán (D9, Price Lock).
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.settlements (
    settlement_id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key             VARCHAR(100)  NOT NULL UNIQUE,
    supplier_id                 UUID          NOT NULL,
    period_start                DATE          NOT NULL,
    period_end                  DATE          NOT NULL,
    revenue_stream              VARCHAR(20)   NOT NULL
                                     CHECK (revenue_stream IN ('ws1_commission','ws2_wholesale','ws3_own_platform')),
    gross_amount                NUMERIC(15,2) NOT NULL,
    platform_fee                NUMERIC(15,2) NOT NULL DEFAULT 0,
    sla_penalty                 NUMERIC(15,2) NOT NULL DEFAULT 0,
    incentive_bonus              NUMERIC(15,2) NOT NULL DEFAULT 0,
    net_payable                 NUMERIC(15,2) NOT NULL,
    commercial_term_version_id  UUID          NOT NULL,
    revenue_split_version_id    UUID          NOT NULL,
    status                      VARCHAR(20)   NOT NULL DEFAULT 'calculated'
                                     CHECK (status IN ('calculated','approved','paid')),
    approved_by                 UUID,
    created_at                  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m6_settlement.settlements (supplier_id, period_start);
CREATE INDEX ON m6_settlement.settlements (status);

-- ----------------------------------------------------------------
-- SETTLEMENT LINES
-- UNIQUE (settlement_id, voucher_id) chặn quyết toán trùng một
-- voucher trong cùng kỳ (CP-5).
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.settlement_lines (
    line_id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    settlement_id          UUID          NOT NULL REFERENCES m6_settlement.settlements,
    voucher_id             UUID          NOT NULL,
    reconciliation_item_id UUID          NOT NULL REFERENCES m6_settlement.reconciliation_items,
    amount                 NUMERIC(15,2) NOT NULL,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT now(),

    UNIQUE (settlement_id, voucher_id)
);

CREATE INDEX ON m6_settlement.settlement_lines (voucher_id);

-- ----------------------------------------------------------------
-- LEDGER ENTRIES (append-only, double-entry)
-- Source of truth cho audit/báo cáo tài chính. Không UPDATE/DELETE.
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.ledger_entries (
    ledger_id       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    correlation_id  UUID          NOT NULL,
    account         VARCHAR(50)   NOT NULL,
    debit           NUMERIC(15,2) NOT NULL DEFAULT 0,
    credit          NUMERIC(15,2) NOT NULL DEFAULT 0,
    reference_type  VARCHAR(50)   NOT NULL,
    reference_id    UUID          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),

    CONSTRAINT debit_or_credit_only CHECK (
        (debit > 0 AND credit = 0) OR (credit > 0 AND debit = 0)
    )
);

CREATE INDEX ON m6_settlement.ledger_entries (correlation_id);
CREATE INDEX ON m6_settlement.ledger_entries (reference_type, reference_id);

-- ----------------------------------------------------------------
-- FRAUD RULES / RULE VERSIONS
-- current_version_id trỏ tới phiên bản active; sửa rule = tạo
-- version mới rồi cập nhật con trỏ (CP-8). Rollback = trỏ lại
-- version cũ trong ≤30 giây, ghi nhận như một version mới.
-- ----------------------------------------------------------------
CREATE TABLE m6_settlement.fraud_rules (
    rule_id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id         UUID,
    current_version_id UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE m6_settlement.fraud_rule_versions (
    version_id  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id     UUID        NOT NULL REFERENCES m6_settlement.fraud_rules,
    version     INT         NOT NULL,
    config      JSONB       NOT NULL,
    reason      TEXT        NOT NULL,
    approved_by UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (rule_id, version)
);

CREATE TABLE m6_settlement.outbox (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(50)  NOT NULL,
    aggregate_id   UUID         NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB        NOT NULL,
    status         VARCHAR(20)  NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','published','failed')),
    retry_count    INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX ON m6_settlement.outbox (status, created_at) WHERE status = 'pending';
```

### M7 Partner — Schema `m7_partner`

```sql
CREATE SCHEMA m7_partner;

-- ----------------------------------------------------------------
-- PARTNERS
-- linked_supplier_id (M1) nullable — quan hệ hai chiều khi một
-- pháp nhân vừa là partner B2B vừa là supplier (D11, M7.A1).
-- documents_complete_at đánh dấu mốc bắt đầu tính SLA onboarding —
-- KHÔNG tính từ lúc submit form (M7.A2).
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.partners (
    partner_id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tax_code                VARCHAR(20)  NOT NULL UNIQUE,
    company_name            VARCHAR(255) NOT NULL,
    partner_type            VARCHAR(20)  NOT NULL
                                 CHECK (partner_type IN ('bank','corporate','airline','loyalty_platform','other')),
    linked_supplier_id      UUID,
    status                  VARCHAR(20)  NOT NULL DEFAULT 'onboarding'
                                 CHECK (status IN (
                                     'onboarding','rejected','approved','active',
                                     'under_watch','suspended','inactive'
                                 )),
    documents_complete_at   TIMESTAMPTZ,
    owner_id                UUID,
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.partners (status);
CREATE INDEX ON m7_partner.partners (linked_supplier_id) WHERE linked_supplier_id IS NOT NULL;

-- ----------------------------------------------------------------
-- PARTNER USERS
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.partner_users (
    user_id    UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID         NOT NULL REFERENCES m7_partner.partners,
    email      VARCHAR(255) NOT NULL,
    role       VARCHAR(20)  NOT NULL CHECK (role IN ('admin','operator','viewer')),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.partner_users (partner_id);

-- ----------------------------------------------------------------
-- CONTRACTS (mutable metadata; KHÔNG sửa trực tiếp khi Active — M7.A3)
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.contracts (
    contract_id    UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_no    VARCHAR(50)  NOT NULL UNIQUE,
    partner_id     UUID         NOT NULL REFERENCES m7_partner.partners,
    contract_type  VARCHAR(20)  NOT NULL
                        CHECK (contract_type IN ('b2b','b2b2c','wholesale','strategic')),
    currency       VARCHAR(3)   NOT NULL DEFAULT 'VND',
    status         VARCHAR(20)  NOT NULL DEFAULT 'draft'
                        CHECK (status IN (
                            'draft','validation','in_review','need_revision',
                            'approved','active','expired','terminated'
                        )),
    effective_from TIMESTAMPTZ,
    effective_to   TIMESTAMPTZ,
    owner_id       UUID         NOT NULL,
    pdf_url        TEXT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.contracts (partner_id, status);
CREATE INDEX ON m7_partner.contracts (effective_from, effective_to);

-- ----------------------------------------------------------------
-- COMMERCIAL TERMS (structured, versioned — D9, D10)
-- Không sửa trực tiếp; mỗi thay đổi tạo version mới với
-- effective_from riêng. scope JSONB dùng cho Term Resolution Engine
-- (D10): specificity_score tính từ số chiều scope đã khai báo.
-- settlement_cycle_days giới hạn 1-90 theo Amendment §2.1.2.
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.commercial_terms (
    term_id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id            UUID          NOT NULL REFERENCES m7_partner.contracts,
    version                INT           NOT NULL,
    scope                  JSONB         NOT NULL,
    specificity_score      INT           NOT NULL DEFAULT 0,
    priority               INT           NOT NULL DEFAULT 0,
    rule_type              VARCHAR(10)   NOT NULL DEFAULT 'allow' CHECK (rule_type IN ('allow','deny')),
    price_basis            JSONB,
    billing_model          VARCHAR(20)   NOT NULL
                                CHECK (billing_model IN (
                                    'immediate','billing_cycle','per_package',
                                    'per_entitlement','usage_based'
                                )),
    payment_terms          JSONB,
    credit_line_limit      NUMERIC(15,2),
    overdue_block_days     INT           NOT NULL DEFAULT 30,
    settlement_cycle_days  INT           NOT NULL DEFAULT 1 CHECK (settlement_cycle_days BETWEEN 1 AND 90),
    revenue_split          JSONB         NOT NULL,
    tax_treatment          JSONB,
    effective_from         TIMESTAMPTZ   NOT NULL,
    effective_to           TIMESTAMPTZ,
    created_by             UUID          NOT NULL,
    approved_by            UUID,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT now(),

    UNIQUE (contract_id, version)
);

CREATE INDEX ON m7_partner.commercial_terms (contract_id, effective_from, effective_to);
CREATE INDEX ON m7_partner.commercial_terms USING GIN (scope);

-- ----------------------------------------------------------------
-- SLA CONFIGS
-- Ba ngưỡng target/warning/breach theo nhóm chỉ số (M7.5.a).
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.sla_configs (
    sla_config_id UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    term_id       UUID          NOT NULL REFERENCES m7_partner.commercial_terms,
    metric_group  VARCHAR(30)   NOT NULL
                      CHECK (metric_group IN (
                          'fulfillment','availability','integration',
                          'reconciliation','customer_service'
                      )),
    target_value  NUMERIC(10,4) NOT NULL,
    warning_value NUMERIC(10,4) NOT NULL,
    breach_value  NUMERIC(10,4) NOT NULL,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.sla_configs (term_id);

-- ----------------------------------------------------------------
-- SLA METRICS (append-only measurement)
-- Ba warning liên tiếp trong cùng kỳ → tự động chuyển breach (edge
-- case M7.8).
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.sla_metrics (
    metric_id      UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    sla_config_id  UUID          NOT NULL REFERENCES m7_partner.sla_configs,
    partner_id     UUID          NOT NULL,
    period_start   DATE          NOT NULL,
    period_end     DATE          NOT NULL,
    measured_value NUMERIC(10,4) NOT NULL,
    status         VARCHAR(10)   NOT NULL CHECK (status IN ('target','warning','breach')),
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.sla_metrics (partner_id, period_start);
CREATE INDEX ON m7_partner.sla_metrics (status) WHERE status IN ('warning','breach');

-- ----------------------------------------------------------------
-- SCORECARDS (định kỳ tháng/quý — M7.5.b)
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.scorecards (
    scorecard_id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id              UUID          NOT NULL REFERENCES m7_partner.partners,
    period_start            DATE          NOT NULL,
    period_end              DATE          NOT NULL,
    revenue_contribution    NUMERIC(15,2),
    sla_score               NUMERIC(5,2),
    fulfillment_rate        NUMERIC(5,2),
    refund_dispute_rate     NUMERIC(5,2),
    reconciliation_quality  NUMERIC(5,2),
    compliance_risk_score   NUMERIC(5,2),
    tier                    VARCHAR(10)   CHECK (tier IN ('platinum','gold','standard','at_risk')),
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.scorecards (partner_id, period_start);

-- ----------------------------------------------------------------
-- ALLOTMENTS
-- Overlay có version trên capacity M1 (D6, D11). Không có cấu hình
-- "vĩnh viễn" — luôn có effective_from/to (M7.7.d).
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.allotments (
    allotment_id   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id     UUID        NOT NULL REFERENCES m7_partner.partners,
    airport_code   VARCHAR(10) NOT NULL,
    service_type   VARCHAR(30) NOT NULL,
    package_id     UUID,
    channel        VARCHAR(20) NOT NULL
                       CHECK (channel IN ('b2b_portal','b2b_sdk','b2c_marketplace')),
    capacity_units INT         NOT NULL CHECK (capacity_units >= 0),
    period_type    VARCHAR(10) NOT NULL CHECK (period_type IN ('day','week','month','quarter','contract')),
    version        INT         NOT NULL DEFAULT 1,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to   TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.allotments (partner_id, airport_code, service_type, effective_from, effective_to);

-- ----------------------------------------------------------------
-- QUOTAS
-- Giới hạn số lượng được mua trong một kỳ (khác allotment: quota
-- không nhất thiết gắn với capacity giữ riêng — M7.7.a).
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.quotas (
    quota_id       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id     UUID        NOT NULL REFERENCES m7_partner.partners,
    scope          JSONB       NOT NULL,
    max_quantity   INT         NOT NULL CHECK (max_quantity > 0),
    used_quantity  INT         NOT NULL DEFAULT 0,
    period_type    VARCHAR(10) NOT NULL CHECK (period_type IN ('day','week','month','quarter')),
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to   TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT quota_used_non_negative CHECK (used_quantity >= 0)
);

CREATE INDEX ON m7_partner.quotas (partner_id, effective_from, effective_to);

-- ----------------------------------------------------------------
-- COMMITMENTS
-- Cam kết tiêu thụ tối thiểu; penalty_terms JSONB áp dụng nếu
-- shortfall cuối kỳ (M7.8 edge case "đạt 95% commitment").
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.commitments (
    commitment_id   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id      UUID        NOT NULL REFERENCES m7_partner.partners,
    min_quantity    INT         NOT NULL CHECK (min_quantity > 0),
    actual_quantity INT         NOT NULL DEFAULT 0,
    period_type     VARCHAR(10) NOT NULL CHECK (period_type IN ('month','quarter','year')),
    penalty_terms   JSONB,
    effective_from  TIMESTAMPTZ NOT NULL,
    effective_to    TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.commitments (partner_id, effective_from, effective_to);

-- ----------------------------------------------------------------
-- COMMUNICATION HUB
-- Trao đổi vận hành ACE ↔ partner; KHÔNG thay thế support/ticket
-- cho consumer cuối (M7.A9).
-- ----------------------------------------------------------------
CREATE TABLE m7_partner.communication_threads (
    thread_id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id            UUID        NOT NULL REFERENCES m7_partner.partners,
    priority              VARCHAR(10) NOT NULL DEFAULT 'normal'
                              CHECK (priority IN ('low','normal','high','urgent')),
    sla_response_minutes  INT         NOT NULL DEFAULT 1440,
    status                VARCHAR(10) NOT NULL DEFAULT 'open' CHECK (status IN ('open','escalated','closed')),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.communication_threads (partner_id, status);

CREATE TABLE m7_partner.communication_messages (
    message_id  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id   UUID        NOT NULL REFERENCES m7_partner.communication_threads,
    sender_id   UUID        NOT NULL,
    sender_type VARCHAR(15) NOT NULL CHECK (sender_type IN ('partner','ace_admin','finance','ops')),
    body        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m7_partner.communication_messages (thread_id, created_at);

CREATE TABLE m7_partner.outbox (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(50)  NOT NULL,
    aggregate_id   UUID         NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB        NOT NULL,
    status         VARCHAR(20)  NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','published','failed')),
    retry_count    INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX ON m7_partner.outbox (status, created_at) WHERE status = 'pending';
```

---

## Các quyết định kiến trúc

### D1 — Bốn database riêng biệt cho M6 (×3) và M7, không shared database

**Quyết định:** M6 Marketplace, M6 Voucher, M6 Settlement và M7 Partner mỗi service có một Amazon Aurora PostgreSQL instance riêng biệt — không phải cluster dùng chung với schema isolation. Mọi tham chiếu liên service (`package_version_id`, `snapshot_id`, `commercial_term_id`, `revenue_split_version_id`...) dùng UUID resolve qua REST API hoặc Kafka event, không bao giờ JOIN xuyên schema/instance.

**Lý do:** M6 Marketplace cần latency thấp cho giao dịch đồng bộ; M6 Voucher cần khả năng hoạt động ngoại tuyến; M6 Settlement cần ACID transaction nghiêm ngặt cho tính toán tài chính — ba đặc tính không thể tối ưu đồng thời trong cùng một database. M7 tách riêng để migration điều khoản thương mại không phụ thuộc vào chu kỳ release của M6.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Backup/restore, failover, migration độc lập theo từng service; Aurora pay-as-you-go scale theo traffic thực tế | Query tổng hợp (ví dụ dashboard toàn bộ order + voucher + settlement) phải qua API composition hoặc read model riêng |
| Sự cố ở Settlement (batch nặng) không ảnh hưởng latency của Marketplace (giao dịch đồng bộ) | Denormalization bắt buộc ở nhiều nơi — ví dụ `order_items` phải tự lưu `commercial_term_version_id` thay vì join M7 |

---

### D2 — Cart Price Lock 15 phút + Quote Snapshot 48 giờ

**Quyết định:** `cart_items.unit_price_locked` khoá giá tại thời điểm add-to-cart, hết hạn sau 15 phút (`price_lock_expires_at`). `quotes.snapshot_data` đóng băng toàn bộ package, số lượng, giá, buyer entity và điều khoản thương mại tại thời điểm tạo quote, hiệu lực 48 giờ. Quote hết hạn hoặc bị huỷ không được dùng lại để tạo order.

**Lý do:** Giá package là kết quả tổng hợp từ M1 (giá entitlement), M2 (margin, pricing rule) và M7 (commercial term) — bất kỳ thành phần nào thay đổi giữa lúc buyer duyệt và lúc checkout đều có thể gây tranh chấp "giá tôi thấy khác giá tôi trả". Khoá tạm thời trong cửa sổ ngắn cân bằng giữa trải nghiệm buyer và rủi ro giá trôi.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Buyer thấy đúng giá đã duyệt trong suốt phiên mua hàng ngắn | Cần job/kiểm tra tại checkout để phát hiện lock hết hạn và yêu cầu re-confirm |
| Quote 48h phù hợp quy trình mua sắm nội bộ B2B cần PO/phê duyệt | Reserve liên quan tới quote hết hạn phải có cơ chế giải phóng, tránh giữ inventory ảo |

---

### D3 — Order Item khoá package_version_id + price_snapshot + idempotency_key

**Quyết định:** `orders.idempotency_key` là UNIQUE bắt buộc theo header `Idempotency-Key` (CP-1). `order_items` khoá `package_version_id`, `price_snapshot`, `commercial_term_version_id` và `revenue_split_version_id` tại thời điểm `order.confirmed` — không tham chiếu package hay hợp đồng "sống".

**Lý do:** Đây là điểm nối dài Price Lock từ ADR-0002 (package_version bất biến) sang M6: order là nơi đầu tiên trong toàn chuỗi giao dịch cần "chốt" đồng thời cả ba nguồn version (M1 gián tiếp qua M2, M2 trực tiếp, M7 trực tiếp). Nếu chỉ khoá package version mà không khoá commercial term version, settlement sau này sẽ tính sai split khi hợp đồng thay đổi giữa lúc order và lúc quyết toán.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Một order_item duy nhất đủ thông tin để settlement tính đúng mà không cần tra cứu lịch sử M7 tại thời điểm quyết toán | Order_item phình thêm 2 cột UUID version — chấp nhận được vì đây là write-once tại confirm |
| Retry do gateway timeout không tạo đơn trùng | Cần đảm bảo idempotency-key được client sinh đúng cách (per checkout attempt, không phải per session) |

---

### D4 — Voucher là snapshot bất biến ký JWT, không tham chiếu package sống

**Quyết định:** `vouchers.snapshot_data` (JSONB) đóng băng giá, entitlement, airport, timeslot, điều kiện sử dụng và redemption_limit tại thời điểm issue. Voucher chỉ tham chiếu `snapshot_id` (M1) và `package_version_id` (M2) — cả hai đều bất biến. QR/token là JWT ký RS256, không chứa PII.

**Lý do:** Supplier có thể sửa metadata dịch vụ sau khi voucher đã phát hành (D2 của ADR-0002); nếu voucher không tự mang snapshot mà chỉ tham chiếu động, nội dung hiển thị cho consumer tại điểm redeem có thể lệch khỏi những gì buyer đã trả tiền mua.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Consumer luôn nhận đúng quyền lợi đã mua dù supplier sửa metadata sau đó | `snapshot_data` trùng lặp dữ liệu đã có ở M1/M2 — chấp nhận được vì đây là bản ghi thương mại, không phải cache |
| Redeem tại kiosk không cần gọi M1/M2 để lấy thông tin hiển thị — hỗ trợ offline (D5) | Snapshot cũ trong voucher đã huỷ vẫn phải giữ để audit — không xoá |

---

### D5 — Two-phase inventory reservation xuyên service + Redemption idempotency

**Quyết định:** Inventory được giữ chỗ (`reserved`) tại M1 khi `order.confirmed` phát ra từ M6 Marketplace; được tiêu thụ thực (`consumed`) khi `voucher.redeemed` phát ra từ M6 Voucher. Đây là mô hình hai giai đoạn xuyên service, dùng lại nguyên tắc atomic UPDATE của ADR-0002 D9 nhưng qua Kafka event thay vì trong cùng transaction. `voucher_redemptions.idempotency_key` là UNIQUE để chống double-redeem khi kiosk offline sync lại.

**Lý do:** Order và redeem xảy ra ở hai service khác nhau (Marketplace và Voucher) và có thể cách nhau vài giờ đến vài ngày. Nếu trừ capacity ngay tại order và trừ tiếp tại redeem, xảy ra double-deduction; nếu chỉ trừ tại redeem, order có thể xác nhận vượt capacity thực trước khi redeem diễn ra.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Không double-deduction xuyên service; không overbooking giữa lúc order và lúc redeem | Cần consumer idempotent ở M1 cho cả hai event (`order.confirmed`, `voucher.redeemed`) — trùng event không được trừ hai lần |
| Redeem tại kiosk hoạt động ngay cả khi mất mạng nhờ JWT + SQLite cục bộ, sync sau với idempotency_key | Cửa sổ giữa lúc mất mạng và lúc sync có sai lệch tồn kho tạm thời (dung sai ≤0,1% theo cam kết) |

---

### D6 — Allotment/Quota/Commitment là overlay có version trên capacity M1

**Quyết định:** `m7_partner.allotments`, `quotas`, `commitments` là các thực thể riêng, có phạm vi (partner × airport × service × channel), thời gian hiệu lực và version — không sửa trực tiếp, đổi bằng amendment. M6 kiểm tra allotment/quota tại 4 checkpoint: browse, add-to-cart (soft), checkout (hard), voucher issuance.

**Lý do:** Capacity thực (M1) và cam kết thương mại (M7) là hai khái niệm khác nhau — capacity 100 ghế có thể chia 30/20/10 buffer/40 free-sell cho các partner khác nhau. Nếu M6 chỉ kiểm tra capacity tổng, một partner có thể vô tình chiếm dụng phần capacity cam kết cho partner khác dù tổng vẫn còn chỗ.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Cam kết hợp đồng về năng lực được thực thi chính xác, không chỉ dựa vào capacity tổng | Mỗi checkout phải kiểm tra thêm một lớp (allotment) ngoài capacity M1 — thêm round-trip hoặc cache |
| Over-allotment vẫn cho giao dịch có kiểm soát (hiển thị `over_allotment_price`, cảnh báo Partner Manager) thay vì chặn cứng | Cần đồng bộ giữa capacity M1 và tổng allotment đã cấp — tổng allotment không được vượt capacity thực |

---

### D7 — Six-state Reconciliation Model với multi-source matching

**Quyết định:** `reconciliation_batches`/`reconciliation_items` dùng chung sáu trạng thái: `pending_data`, `matched`, `mismatch`, `pending_review`, `adjusted`, `reconciled`. Chỉ record `reconciled` hoặc `adjusted` (đã approved) mới được dùng làm input cho `settlements`. `variance_amount` là generated column từ `ace_amount - supplier_amount`; khi khác 0, hệ thống tự động chặn payout liên quan và mở ticket.

**Lý do:** Dữ liệu redeem có tối đa 5 nguồn (ACE, NCC, kiosk, file upload, adjustment) không đảm bảo luôn khớp nhau. Nếu quyết toán được phép chạy trên dữ liệu `pending_data` hoặc `mismatch`, ACE có rủi ro trả sai tiền cho NCC mà không có cơ sở đối chiếu khi tranh chấp.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Không có con đường nào để dữ liệu chưa khớp lọt vào settlement — gate cứng ở tầng DB (`settlement_lines` tham chiếu `reconciliation_item_id`) | Cần vận hành thủ công cho `mismatch`/`pending_review` — không tự động hoá 100% được vì cần Finance/NCC xác nhận |
| Generated column `variance_amount` loại bỏ sai sót tính tay khi Finance rà soát | NCC chưa upload trước hạn T+n vẫn giữ ở `pending_data` — cần cảnh báo chủ động, không được coi là lỗi im lặng |

---

### D8 — Settlement append-only ledger + Ba công thức doanh thu (WS1/WS2/WS3) + Two-Ring Settlement

**Quyết định:** `settlements` và `ledger_entries` là bảng append-only (chỉ INSERT). `settlements.revenue_stream` phân biệt ba luồng WS1 (hoa hồng), WS2 (bán sỉ), WS3 (tự doanh) với công thức tính `net_payable` khác nhau (dùng `shopspring/decimal`, không dùng floating-point). Two-Ring Settlement tách Ring 1 (ACE ↔ NCC, tham chiếu M1) và Ring 2 (ACE ↔ B2B Partner, tham chiếu M7) đối soát chéo theo `order_item_id`/`voucher_id`.

**Lý do:** Ba mô hình kinh doanh có logic chia tiền khác nhau — gộp chung một công thức sẽ buộc phải nhồi nhét điều kiện rẽ nhánh vào application code thay vì thể hiện rõ ràng trong dữ liệu. Ledger append-only là yêu cầu kế toán bắt buộc (bút toán kép) — sửa lịch sử tài chính trực tiếp là hành vi không thể chấp nhận với kiểm toán.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Mỗi luồng doanh thu minh bạch, dễ kiểm toán độc lập; thêm luồng mới (Phase sau) không phá vỡ luồng cũ | Ba công thức phải được đồng bộ chính xác giữa migration DB (CHECK/generated column) và code Go — divergence gây sai số |
| Ledger double-entry là bằng chứng không thể chối cãi cho mọi tranh chấp tài chính | Bảng tăng trưởng không giới hạn — cần chiến lược partition/archival tương tự `price_history`/`audit_log` ở ADR-0002 |

---

### D9 — Contract/Commercial Terms hai lớp dữ liệu với versioning bắt buộc

**Quyết định:** `contracts` lưu metadata (mutable khi chưa Active). `commercial_terms` là structured data có version, `UNIQUE (contract_id, version)`, không sửa trực tiếp khi hợp đồng Active — mọi thay đổi giá, SLA, revenue split, settlement cycle, allotment hay tax treatment đều tạo version mới với `effective_from` riêng. File PDF chỉ lưu qua `contracts.pdf_url`, không thay thế dữ liệu vận hành.

**Lý do:** Đây là nền tảng của Price Lock xuyên M7 → M2 → M6: order/voucher/invoice/settlement phải tham chiếu đúng `term_id` (version) đang hiệu lực tại thời điểm giao dịch, không phải version hiện hành của hợp đồng. Nếu commercial terms có thể UPDATE tại chỗ, mọi giao dịch lịch sử sẽ bị tính lại ngầm định — vi phạm CP-3 (tính toán không hồi tố).

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Giao dịch cũ giữ nguyên điều khoản cũ dù hợp đồng đã renegotiate — tranh chấp có câu trả lời dứt khoát | Mỗi thay đổi nhỏ (kể cả sửa lỗi chính tả trong điều khoản) đều cần version mới + quy trình phê duyệt |
| M2 và M6 đọc trực tiếp dữ liệu có cấu trúc, không cần parse PDF mỗi lần tính giá | Cần review kỹ giữa PDF và structured data — sai lệch giữa hai nguồn phải được cảnh báo, không tự động đồng bộ |

---

### D10 — Term Resolution Engine: specificity > priority > deny-wins > latest version

**Quyết định:** Khi nhiều `commercial_terms` cùng match một giao dịch, hệ thống resolve theo thứ tự cố định: (1) phạm vi cụ thể hơn thắng phạm vi chung — đo bằng `specificity_score` tính từ số chiều đã khai báo trong `scope` JSONB; (2) nếu cùng độ cụ thể, `priority` cao hơn thắng; (3) nếu xung đột `allow`/`deny`, `deny` thắng; (4) nếu vẫn còn nhiều version hợp lệ, version có `effective_from` mới nhất còn hiệu lực tại thời điểm giao dịch thắng. Không resolve được thì raise alert cho Partner Manager thay vì tự chọn ngẫu nhiên.

**Lý do:** Term Resolution là engine dùng chung cho nhiều điểm quyết định khác nhau trong M2/M6 (channel visibility, giá, billing model, SLA, settlement cycle) — nếu không có thứ tự resolve xác định và nhất quán, các module khác nhau có thể suy luận ra kết quả khác nhau cho cùng một giao dịch.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Một quy tắc resolve duy nhất áp dụng nhất quán ở mọi điểm quyết định (browse, checkout, invoice, settlement) | `specificity_score` cần thuật toán tính rõ ràng và version hoá cùng schema — thay đổi công thức tính điểm có thể đổi kết quả resolve của giao dịch cũ nếu không cẩn thận |
| `deny` mặc định thắng `allow` an toàn hơn cho ACE khi có xung đột không lường trước | Trường hợp không resolve được cần escalation thủ công — không thể để trống hoặc mặc định im lặng |

---

### D11 — Partner và Supplier tách hồ sơ (dual identity) với quan hệ hai chiều

**Quyết định:** `m7_partner.partners.linked_supplier_id` là UUID nullable trỏ sang `m1_supply.suppliers.supplier_id` (không FK). Một pháp nhân vừa là B2B partner vừa là supplier có hai `partner_id`/`supplier_id` riêng, hai `contract` riêng (B2B và supplier), và hai luồng tài chính riêng (Ring 2 với vai trò partner, Ring 1 với vai trò supplier ở M6 Settlement).

**Lý do:** Lẫn lộn vai trò là rủi ro nghiệp vụ phổ biến nhất của M7 (ví dụ VietJet vừa mua voucher cho khách vừa cung cấp dịch vụ priority boarding). Nếu dùng chung một hồ sơ, nghĩa vụ tài chính hai chiều (ACE nợ VietJet tiền supplier, VietJet nợ ACE tiền mua voucher) sẽ bị trộn lẫn trong cùng một ledger, gây sai số dư ròng và khó giải trình khi kiểm toán.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Hai luồng tài chính (ring 1/ring 2) không bao giờ bị cấn trừ nhầm vào nhau | Cần đồng bộ thủ công hoặc qua sự kiện khi thông tin pháp nhân dùng chung (địa chỉ, người đại diện) thay đổi ở một bên |
| Truy vết rõ ràng: mọi giao dịch biết chính xác đang ở vai trò nào | UI/portal cần phân biệt rõ ngữ cảnh (đang thao tác với tư cách partner hay supplier) để tránh nhầm lẫn thao tác |

---

### D12 — Transactional Outbox Pattern tái sử dụng trên toàn bộ M6/M7

**Quyết định:** Cả bốn service (M6 Marketplace, M6 Voucher, M6 Settlement, M7 Partner) đều có bảng `outbox` với cấu trúc giống hệt bảng đã định nghĩa ở ADR-0002 D11. Mọi state transition quan trọng (`order.confirmed`, `voucher.issued`, `voucher.redeemed`, `settlement.calculated`, `contract.activated`...) đều INSERT vào `outbox` trong cùng transaction với thay đổi entity, được Outbox Relay poll và publish lên Kafka với `SELECT ... FOR UPDATE SKIP LOCKED`.

**Lý do:** Rủi ro mất event giữa lúc commit DB và publish Kafka đã được phân tích chi tiết ở ADR-0002 (Thách thức 5) cho M1/M2; rủi ro này áp dụng y hệt cho M6/M7 vì tất cả đều dùng Aurora PostgreSQL + Amazon MSK không có distributed transaction. Việc voucher.redeemed bị mất sẽ khiến settlement không bao giờ được kích hoạt cho giao dịch đó — hậu quả tài chính trực tiếp.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Nhất quán thiết kế trên cả 6 service của nền tảng — team vận hành một pattern duy nhất, không phải học 2 cách khác nhau | `outbox` nhân lên 4 lần (một bảng mỗi service) — cần job dọn định kỳ ở cả 4 nơi |
| Đường đi trọng yếu `voucher.redeemed → Settlement Consumer` dùng Kafka transaction để đọc + xử lý + commit offset cùng lúc với ghi DB — đảm bảo exactly-once cho financial record | Cần alert riêng theo từng service khi `retry_count > 3`, không thể dùng chung một dashboard nếu không tổng hợp theo `aggregate_type` |

---

### D13 — SLA/Scorecard versioned với ba ngưỡng và cascade sang trạng thái partner

**Quyết định:** `sla_configs` định nghĩa ba ngưỡng (`target_value`, `warning_value`, `breach_value`) theo từng nhóm chỉ số, gắn với `commercial_terms` (nên tự động version hoá theo hợp đồng). `sla_metrics` là bảng đo lường append-only theo kỳ. Vi phạm `breach` tạo ticket và có thể chuyển `partners.status` sang `under_watch`; ba lần `warning` liên tiếp trong cùng kỳ tự động nâng thành `breach`.

**Lý do:** SLA phải là dữ liệu có cấu trúc để hệ thống tự động cảnh báo và ảnh hưởng tới quyết định quản trị đối tác (renewal, credit line, suspend) — không thể là con số nằm rải rác trong báo cáo thủ công. Gắn SLA config với version của commercial terms đảm bảo khi hợp đồng renegotiate SLA, các vi phạm cũ vẫn được đánh giá theo ngưỡng cũ (nhất quán với D9).

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Scorecard trở thành cơ sở ra quyết định tự động hoá một phần (cảnh báo, ticket) thay vì chỉ là báo cáo tham khảo | Cần job định kỳ tổng hợp `sla_metrics` thành `scorecards` theo tháng/quý — thêm một background process cần giám sát |
| Quy tắc "3 warning → 1 breach" giảm tải xử lý thủ công cho vi phạm nhẹ lặp lại | Ngưỡng threshold cứng có thể cần tinh chỉnh theo thực tế vận hành Phase 1 — nên để cấu hình được, không hard-code |

---

## Hệ quả

| Thách thức | Đảm bảo | Lưu ý |
|------------|---------|-------|
| **Tính bất biến xuyên 4 module** | Snapshot chain (service_snapshot → package_version → cart/quote lock → order_item → voucher → settlement) đảm bảo mọi bước giao dịch dùng đúng version tại thời điểm phát sinh; thay đổi thượng nguồn không hồi tố. | `order_items` phải lưu đủ 2 UUID version (commercial_term, revenue_split) — thiếu một trong hai sẽ làm settlement không thể tính đúng khi hợp đồng đã đổi. |
| **Idempotency toàn tuyến** | Idempotency-Key bắt buộc + composite UNIQUE key ở DB cho order, voucher issuance, redemption và settlement loại bỏ giao dịch trùng trong mọi kịch bản retry, kể cả sync ngoại tuyến. | Cần chuẩn hoá cách client sinh idempotency-key (per attempt, không phải per session) — sai cách sinh key làm mất tác dụng bảo vệ. |
| **Kiểm soát năng lực đa tầng** | Allotment/quota/commitment là overlay có version trên capacity M1, kiểm tra tại 4 checkpoint của M6 — cam kết hợp đồng về năng lực được thực thi đúng dù capacity tổng còn dư. | Cần job đối soát tổng allotment đã cấp không vượt capacity thực của M1 — dangling allotment (capacity M1 giảm nhưng allotment chưa điều chỉnh) cần audit định kỳ. |
| **Toàn vẹn tài chính đối soát–quyết toán** | Six-state reconciliation gate + append-only ledger double-entry + ba công thức WS1/WS2/WS3 tách bạch đảm bảo chỉ dữ liệu đã khớp mới vào settlement, và mọi điều chỉnh đều có bút toán bù trừ truy vết được. | Auto-payout là Phase sau (CP-2) — Phase 1 dừng ở settlement statement/payment instruction cho Finance xử lý ngoài hệ thống. |
| **Hợp đồng có cấu trúc & resolve xung đột** | Commercial terms versioned + Term Resolution Engine (specificity > priority > deny > latest) đảm bảo mọi điểm quyết định trong M2/M6 dùng cùng một quy tắc, không suy luận khác nhau cho cùng giao dịch. | Trường hợp không resolve được phải escalate cho Partner Manager, không được mặc định — cần UI hỗ trợ xử lý nhanh để không chặn giao dịch buyer quá lâu. |
| **Độ tin cậy ngoại tuyến & message publishing** | JWT xác thực cục bộ + SQLite kiosk cache đảm bảo redeem hoạt động khi mất mạng; Transactional Outbox trên cả 4 service đảm bảo không mất event dù crash tại bất kỳ điểm nào. | Consumer phải idempotent ở mọi service downstream (đặc biệt M1 nhận cả `order.confirmed` và `voucher.redeemed`); cần alert khi `outbox.retry_count > 3` ở cả 4 nơi. |

**Hệ quả bổ sung:**

- **M6 và M7 phụ thuộc ngược vào ADR-0002:** `order_items.package_version_id` và `vouchers.snapshot_id` là tham chiếu trực tiếp tới các bảng bất biến đã thiết kế ở M1/M2. Bất kỳ thay đổi contract shape nào ở `GET /v1/services/{id}/snapshots` hay `package_versions.config_snapshot` đều ảnh hưởng trực tiếp tới M6 — cần đồng bộ version hoá API giữa hai ADR.
- **M7 là single source of truth cho mọi con số thương mại dùng ở M2 và M6:** `commercial_terms.revenue_split`, `billing_model`, `settlement_cycle_days` được đọc trực tiếp, không parse PDF. Nguyên tắc này (M7.6.a) không có ngoại lệ — mọi user story mới liên quan giá/billing/settlement phải hỏi "trường này đã có trong commercial_terms structured chưa?" trước khi thêm logic mới.
- **Two-Ring Settlement là ràng buộc thiết kế cứng, không phải tuỳ chọn:** bất kỳ entity nào vừa đóng vai trò supplier vừa là partner đều bắt buộc có hai hồ sơ, hai hợp đồng, hai luồng tài chính (D11) — nhầm lẫn ở đây là rủi ro tài chính/pháp lý cao nhất trong cả bốn module theo T1.A.
- **Outbox pattern nhân rộng nhất quán:** với D12, toàn bộ 6 service của nền tảng (M1, M2, M6×3, M7) dùng chung một pattern publish sự kiện — đơn giản hoá vận hành nhưng cũng có nghĩa lỗi thiết kế trong pattern này (nếu có) sẽ nhân bản ra toàn hệ thống; mọi thay đổi vào outbox relay logic cần test trên cả 6 service trước khi rollout.
