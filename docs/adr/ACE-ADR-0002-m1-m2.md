# ADR-0002: Kiến trúc và Thiết kế Database M1 Supply Service & M2 Package Service

---

## Bối cảnh

M1 và M2 tạo thành lớp cung ứng và lớp sản phẩm của nền tảng ACE. Hai module này cùng trả lời hai câu hỏi:

- **M1:** Dịch vụ nào đang tồn tại, ai cung cấp, giá bao nhiêu, còn bao nhiêu năng lực phục vụ?
- **M2:** Các dịch vụ đó được đóng gói thành sản phẩm bán được như thế nào, định giá ra sao, và hiển thị cho đúng người mua nào?

Năm thách thức cốt lõi chi phối mọi quyết định kiến trúc và thiết kế database trong tài liệu này:

1. **Tính bất biến** — giá, định nghĩa dịch vụ và cấu hình gói được thoả thuận tại thời điểm giao dịch không bao giờ được thay đổi hồi tố, dù nhà cung ứng có chỉnh sửa metadata hay giá thay đổi về sau.
2. **Thương mại hoá có kiểm soát** — không có dịch vụ hay gói nào đến tay người mua mà không qua cổng phê duyệt thuộc ACE; nhà cung ứng và Product Manager không thể tự publish thẳng ra marketplace.
3. **Tính toàn vẹn tài chính** — revenue split giữa ACE và từng nhà cung ứng phải được khoá tại thời điểm bán và có thể truy vết về đúng điều khoản thương mại hiệu lực tại thời điểm đó; tham chiếu liên service dùng UUID thay vì FK để M1 và M2 deploy độc lập.
4. **Hiệu năng ở quy mô lớn** — từ Phase 1 (một vài sân bay) đến Phase 4 (đa quốc gia, > 10M rows) mà không cần viết lại schema hay query.
5. **Tính tin cậy của message publishing** — sự kiện Kafka (`price.changed`, `package.published`) phải được đảm bảo gửi đúng một lần; crash giữa chừng không được làm mất event hoặc để hệ thống rơi vào trạng thái không nhất quán giữa DB và message broker.

---

## Phân tích thách thức

### Thách thức 1 — Tính bất biến

**Vấn đề:** Nhà cung ứng chỉnh sửa mô tả dịch vụ, thay đổi giá, hoặc ngừng kích hoạt một dịch vụ sau khi các gói và voucher đã được phát hành. Nếu downstream records tham chiếu tới thực thể đang sống, bất kỳ chỉnh sửa nào từ phía trên đều âm thầm làm sai lệch nội dung người mua đã mua. Tương tự, nếu bảng giá cho phép UPDATE, không có cách nào chứng minh giá nào đang hiệu lực tại thời điểm một giao dịch cụ thể xảy ra trong quá khứ.

**Giải pháp — Mô hình thực thể hai tầng + price history append-only:**

```
  Nhà cung ứng chỉnh sửa tự do
        │
        ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  services  (mutable — có thể sửa)                           │
  │  • tên, mô tả, hình ảnh, điều kiện: chỉnh sửa tự do         │
  │  • KHÔNG được tham chiếu trực tiếp bởi package hay voucher  │
  └────────────────────────────┬────────────────────────────────┘
                               │  ACE phê duyệt → publish
                               ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  service_snapshots  (immutable — bất biến, append-only)     │
  │  • bản sao đầy đủ đóng băng lúc publish (snapshot_data)     │
  │  • snapshot_id = UUID cố định; version v+1 mỗi lần publish  │
  │  • snapshot cũ không bao giờ bị sửa hoặc xoá                │
  │  • ĐÂY mới là thứ package và voucher tham chiếu             │
  └────────────────────────────┬────────────────────────────────┘
                               │
  Tương tự với giá:            │
  ┌─────────────────────────────────────────────────────────────┐
  │  price_history  (append-only)                               │
  │  ┌──────────┬───────────────┬─────────────┬────────────┐    │
  │  │ price_id │ effective_from│ effective_to│   amount   │    │
  │  ├──────────┼───────────────┼─────────────┼────────────┤    │
  │  │  uuid-1  │  2024-01-01   │ 2024-06-01  │  100.000   │    │  ← không xoá
  │  │  uuid-2  │  2024-06-01   │    NULL     │  120.000   │    │  ← row mới
  │  └──────────┴───────────────┴─────────────┴────────────┘    │
  └─────────────────────────────────────────────────────────────┘

  Tương tự ở M2:
  packages (mutable) → publish → package_versions (immutable)
  order_items.package_version_id = version_id tại checkout ← KHOÁ
```

Quyết định: **D2**, **D3**, **D6**

---

### Thách thức 2 — Thương mại hoá có kiểm soát

**Vấn đề:** Nhà cung ứng có thể khai báo bất kỳ dịch vụ nào. PM có thể cấu hình bất kỳ gói nào. Nếu không có cơ chế kiểm soát, dữ liệu chưa review sẽ đến tay người mua — giá sai, thông tin nhà cung ứng chưa xác minh, gói hiển thị sai đối tác.

**Giải pháp — Cổng phê duyệt bắt buộc và cascade khi upstream thay đổi:**

```
  M1 — Vòng đời nhà cung ứng
  pending_profile ──▶ pending_review ──▶ needs_update
                            │
                            ▼
                        active ──▶ suspended ──▶ inactive

  M1 — Vòng đời dịch vụ
  draft ──▶ pending_review ──▶ needs_update
                  │
                  ▼
              approved ──▶ active ──▶ paused ──▶ deprecated ──▶ archived

  M2 — Vòng đời gói dịch vụ
  draft ──▶ pending_review ──▶ active ──▶ paused
                                      └──▶ replaced ──▶ archived

  Cascade khi upstream thay đổi (qua Kafka):
  service price changes  ──▶  package liên quan → draft (tạm dừng bán)
  service paused/depr.   ──▶  package liên quan → paused + cảnh báo PM
```

Quyết định: **D4**, **D5**

---

### Thách thức 3 — Tính toàn vẹn tài chính và tham chiếu liên service

**Vấn đề:** Một gói bundling entitlement từ nhiều nhà cung ứng — mỗi entitlement có platform fee, ACE margin và VAT riêng. Điều khoản thương mại thay đổi giữa chừng. Nếu revenue split không được khoá tại thời điểm giao dịch, settlement sẽ bị lệch. Đồng thời, nếu M2 dùng FK xuyên schema để tham chiếu M1, hai service bị couple về migration — không thể deploy độc lập.

**Giải pháp — Revenue split versioned + UUID cross-reference:**

```
  Mô hình 5 thành phần cho mỗi dòng entitlement:
  ┌──────────────────────────────────────────────────────────────┐
  │  Giá bán = NCC share + Platform fee + ACE margin             │
  │            + VAT(NCC) + VAT(ACE)                             │
  │                                                              │
  │  platform_fee_pct + ace_margin_pct + ncc_share_pct = 100%    │
  │  Bắt buộc bởi CHECK constraint tại DB — không thể thiếu.     │
  └──────────────────────────────────────────────────────────────┘

  Tham chiếu liên service:
  ┌────────────────────────────┐       ┌──────────────────────────────┐
  │  m1_supply                 │       │  m2_package                  │
  │  service_snapshots         │       │  package_items               │
  │  PK: snapshot_id UUID      │       │  snapshot_id UUID ───────────┼──▶ UUID
  └────────────────────────────┘       │                              │    resolve
                                       │                              │    qua API
  Không có FK xuyên schema.            └──────────────────────────────┘    call
  Validate sự tồn tại ở application layer trước khi publish package.
```

Quyết định: **D1**, **D7**, **D8**

---

### Thách thức 4 — Hiệu năng ở quy mô lớn

**Vấn đề:** `services`, `inventory`, `audit_log` sẽ vượt 10 triệu rows khi ACE mở rộng. Query không có partition phải scan toàn bộ bảng dù chỉ cần dữ liệu một sân bay. `inventory` cần cập nhật `capacity_used` từ nhiều checkout đồng thời mà không bị race condition dẫn đến overbooking.

**Giải pháp — Partitioning + atomic UPDATE:**

```
  Partitioning:
  services, inventory ──▶ PARTITION BY LIST (airport_code)
                          query SGN chỉ scan partition SGN
  audit_log, price_history ──▶ PARTITION BY RANGE (created_at) monthly
                               DROP partition cũ thay vì DELETE

  Atomic capacity update (chặn overbooking):
  ┌─────────────────────────────────────────────────────────────┐
  │  UPDATE inventory                                           │
  │  SET capacity_used = capacity_used + $qty                   │
  │  WHERE slot_id = $1                                         │
  │    AND capacity_used + $qty <=                              │
  │        FLOOR(capacity_total * (1 + overbook_pct / 100.0))   │
  │  RETURNING slot_id;                                         │
  │  -- 0 rows → hết chỗ; 1 row → thành công                    │
  └─────────────────────────────────────────────────────────────┘
```

Quyết định: **D9**, **D10**

---

### Thách thức 5 — Tính tin cậy của message publishing

**Vấn đề:** M1 và M2 phát sinh Kafka events tại các điểm chuyển trạng thái quan trọng (`price.changed`, `package.published`, và cascade `service.paused`/`service.deprecated`). Cách triển khai naive — commit DB trước, publish Kafka sau — tạo khoảng thời gian nguy hiểm:

```
  UPDATE services SET status='paused'
  tx.Commit()                        ← DB đã ghi, không rollback được
  kafka.Publish("service.paused")    ← crash tại đây → message mất vĩnh viễn

  Hệ quả: M2 không biết service đã paused
          → package chứa service này vẫn hiển thị active, tiếp tục bán
          → buyer mua phải entitlement không còn khả dụng
```

Publish Kafka trước, commit DB sau cũng không giải quyết được — nếu DB commit fail, event đã đến consumer và gây side-effect không có bản ghi tương ứng.

**Giải pháp — Transactional Outbox Pattern:**

```
  Trong cùng 1 DB transaction:
  ┌──────────────────────────────────────────────────────────┐
  │  BEGIN                                                   │
  │  UPDATE services SET status = 'paused'                   │
  │  INSERT INTO outbox (event_type, payload)                │
  │         VALUES ('service.paused', '{...}')               │
  │  COMMIT  ← atomic: cả hai cùng thành công hoặc cùng fail │
  └──────────────────────────────────────────────────────────┘
           │
           │  (DB đã commit, outbox row tồn tại)
           ▼
  ┌──────────────────────────────────────────────────────────┐
  │  Outbox Relay (background goroutine)                     │
  │  SELECT ... FROM outbox WHERE status='pending'           │
  │  FOR UPDATE SKIP LOCKED  ← nhiều instance không conflict │
  │                                                          │
  │  kafka.Publish(event)                                    │
  │  UPDATE outbox SET status='published'                    │
  └──────────────────────────────────────────────────────────┘
           │
           ▼  (có thể publish trùng nếu relay crash sau publish
               nhưng trước khi mark 'published')
  ┌──────────────────────────────────────────────────────────┐
  │  Consumer phải idempotent                                │
  │  INSERT INTO processed_events (event_id)                 │
  │  ON CONFLICT DO NOTHING  ← skip nếu đã xử lý             │
  └──────────────────────────────────────────────────────────┘
```

**Phạm vi áp dụng:** Outbox/Kafka chỉ cần cho các sự kiện cascade — nơi consumer phải tự động phản ứng mà không có ai chủ động hỏi (`price.changed`, `service.paused`, `service.deprecated`, `package.published`). `service.approved` không thuộc nhóm này: bước duy nhất tiêu thụ nó là PM chủ động chọn snapshot khi xây dựng gói (`M2: PM xây dựng gói → chọn snapshots đã duyệt từ M1`) — một thao tác pull đồng bộ qua `GET /v1/services/{id}/snapshots`, không cần độ trễ thời gian thực hay đảm bảo delivery của message broker. Vì vậy service approval không phát Kafka event; M2 đọc snapshot trực tiếp qua API khi cần.

Quyết định: **D11**

---

## Luồng hệ thống tổng thể

```
 ┌────────────────────────────────────────────────────────────────────────────┐
 │                         M1 + M2 — LUỒNG NGHIỆP VỤ                          │
 └────────────────────────────────────────────────────────────────────────────┘

  ┌──────────────┐
  │ Nhà cung ứng │
  └──────┬───────┘
         │ 1. tự đăng ký hoặc ACE mời
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M1: Onboarding — thu thập pháp nhân, MST, ngân hàng,        │
  │  airport_scope, sync_mode, hồ sơ pháp lý                     │
  └──────────────────────┬───────────────────────────────────────┘
         │ 2. ACE Admin review KYC (thủ công)
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M1: Supplier → active  │  event: supplier.onboarded         │
  └──────────────────────┬───────────────────────────────────────┘
         │ 3. nhà cung ứng khai báo dịch vụ
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M1: Service review — tên, loại, sân bay, giá, capacity      │
  │  INSERT price_history row (append-only)                      │
  │  Tạo inventory slots                                         │
  └──────────────────────┬───────────────────────────────────────┘
         │ 4. ACE Admin phê duyệt
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M1: service_snapshot tạo (bất biến, v1)                     │
  │  KHÔNG phát Kafka event — snapshot sẵn sàng qua API           │
  └──────────────────────┬───────────────────────────────────────┘
         │ snapshot_id truy vấn qua GET /v1/services/{id}/snapshots
         │ khi PM chủ động xây dựng gói (pull, không cần realtime)
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M2: PM xây dựng gói                                         │
  │  1. chọn snapshots đã duyệt từ M1                            │
  │  2. đặt phạm vi: sân bay, kênh, thời gian hiệu lực           │
  │  3. cấu hình giá + margin (sàn margin được kiểm soát)        │
  │  4. đặt targeting rules (allow/deny, tối đa 5 cấp)           │
  │  5. cấu hình revenue split theo từng dòng entitlement        │
  │  6. thêm khuyến mại, preview targeting                       │
  └──────────────────────┬───────────────────────────────────────┘
         │ 5. submit → ACE phê duyệt
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M2: package_version tạo (bất biến)                          │
  │  revenue_split version khoá                                  │
  │  event: package.published → Kafka                            │
  └──────────────────────┬───────────────────────────────────────┘
         │ version_id sẵn sàng cho M6 catalog
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  M6 Marketplace (downstream — ngoài phạm vi ADR này)         │
  └──────────────────────────────────────────────────────────────┘


 ┌────────────────────────────────────────────────────────────────────────────┐
 │                     CASCADE KHI UPSTREAM THAY ĐỔI                          │
 └────────────────────────────────────────────────────────────────────────────┘

  event: price.changed (Kafka)
  ──▶ M2 consumer: package chứa snapshot_id → draft, tạm dừng bán

  event: service.paused (Kafka)
  ──▶ M2 consumer: package chứa snapshot_id → paused, cảnh báo PM


 ┌────────────────────────────────────────────────────────────────────────────┐
 │                              ERD                                           │
 └────────────────────────────────────────────────────────────────────────────┘

  M1 (schema: m1_supply)
  suppliers ──< supplier_users
  suppliers ──< services ──────< service_snapshots  (immutable)
                         ──────< price_history       (append-only)
                         ──────< inventory ──────────< reservations (TTL 15p)
  audit_log
  outbox  (relay → Kafka)

  M2 (schema: m2_package)
  packages ──< package_versions (immutable) ──< package_items  ──▶ M1 snapshot_id
                                            ──< prices
                                            ──< pricing_rules  ──▶ M7 term_id
                                            ──< revenue_splits
                                            ──< targeting_rules
           ──< promotions
           ──< ab_tests
  audit_log
```

---

## DDL

### M1 — Schema `m1_supply`

```sql
CREATE SCHEMA m1_supply;

-- btree_gist bắt buộc cho EXCLUDE USING GIST kết hợp cột equality (=)
-- với range overlap (&&) — dùng trong price_history bên dưới.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- ----------------------------------------------------------------
-- SUPPLIERS
-- tax_code không được sửa sau khi KYC duyệt (enforce ở app layer).
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.suppliers (
    supplier_id        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tax_code           VARCHAR(20)  NOT NULL UNIQUE,
    company_name       VARCHAR(255) NOT NULL,
    legal_rep_name     VARCHAR(255),
    address            TEXT,
    invoice_info       JSONB,
    contact_ops        JSONB,
    contact_finance    JSONB,
    contact_tech       JSONB,
    bank_account       JSONB,
    airport_scope      VARCHAR(10)[],
    service_categories VARCHAR(30)[],
    sync_mode          VARCHAR(20)  NOT NULL DEFAULT 'manual'
                           CHECK (sync_mode IN ('manual','bulk_upload','api')),
    status             VARCHAR(30)  NOT NULL DEFAULT 'pending_profile'
                           CHECK (status IN (
                               'pending_profile','pending_review','needs_update',
                               'active','suspended','inactive'
                           )),
    kyc_approved_by    UUID,
    kyc_approved_at    TIMESTAMPTZ,
    suspended_reason   TEXT,
    opid_ref           VARCHAR(100),
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m1_supply.suppliers (status);
CREATE INDEX ON m1_supply.suppliers USING GIN (airport_scope);

-- ----------------------------------------------------------------
-- SUPPLIER USERS
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.supplier_users (
    user_id        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id    UUID         NOT NULL REFERENCES m1_supply.suppliers,
    email          VARCHAR(255) NOT NULL,
    role           VARCHAR(20)  NOT NULL CHECK (role IN ('admin','operator','viewer')),
    status         VARCHAR(20)  NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active','inactive')),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deactivated_at TIMESTAMPTZ
);

CREATE INDEX ON m1_supply.supplier_users (supplier_id);

-- ----------------------------------------------------------------
-- SERVICES (mutable)
-- Không được tham chiếu trực tiếp bởi package/voucher.
-- Chưa partition ở Phase 1 (D10) — PARTITION BY LIST (airport_code)
-- yêu cầu airport_code nằm trong PK, việc này sẽ đổi shape của mọi FK
-- trỏ vào service_id (service_snapshots, price_history, inventory) nên
-- được để lại cho migration riêng khi thực sự vượt ngưỡng 10M rows.
-- Index trên airport_code bên dưới là giải pháp tạm thời.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.services (
    service_id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id         UUID         NOT NULL REFERENCES m1_supply.suppliers,
    service_type        VARCHAR(30)  NOT NULL
                            CHECK (service_type IN (
                                'lounge','fast_track','fnb','transport','baggage'
                            )),
    name_vi             VARCHAR(255) NOT NULL,
    name_en             VARCHAR(255) NOT NULL,
    description_vi      TEXT,
    description_en      TEXT,
    conditions_vi       TEXT,
    conditions_en       TEXT,
    images              JSONB,
    airport_code        VARCHAR(10)  NOT NULL,
    location_detail     VARCHAR(255),
    status              VARCHAR(30)  NOT NULL DEFAULT 'draft'
                            CHECK (status IN (
                                'draft','pending_review','needs_update','approved',
                                'active','paused','deprecated','archived'
                            )),
    submitted_at        TIMESTAMPTZ,
    approved_by         UUID,
    approved_at         TIMESTAMPTZ,
    reject_reason       TEXT,
    current_snapshot_id UUID,
    opid_ref            VARCHAR(100),
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m1_supply.services (supplier_id, status);
CREATE INDEX ON m1_supply.services (service_type);
CREATE INDEX ON m1_supply.services (airport_code);

-- ----------------------------------------------------------------
-- SERVICE SNAPSHOTS (immutable)
-- Package và voucher LUÔN trỏ vào snapshot_id, không phải service_id.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.service_snapshots (
    snapshot_id    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id     UUID        NOT NULL REFERENCES m1_supply.services,
    version        INT         NOT NULL,
    snapshot_data  JSONB       NOT NULL,
    published_by   UUID        NOT NULL,
    published_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (service_id, version)
);

-- current_snapshot_id là con trỏ cùng schema (không phải cross-service
-- như D1 cấm) nên được FK hoá bình thường để tránh dangling reference.
ALTER TABLE m1_supply.services
    ADD CONSTRAINT fk_services_current_snapshot
    FOREIGN KEY (current_snapshot_id) REFERENCES m1_supply.service_snapshots (snapshot_id);

-- ----------------------------------------------------------------
-- PRICE HISTORY (append-only)
-- Khi giá thay đổi: INSERT row mới, KHÔNG UPDATE row cũ.
-- EXCLUDE GIST (cần extension btree_gist ở trên) chặn hai specific
-- price chồng lấp thời gian. Chưa partition ở Phase 1 (D10): RANGE BY
-- created_at sẽ xung đột với EXCLUDE constraint hiện tại (không định
-- nghĩa trên created_at) — cần thiết kế lại khi thực sự cần partition.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.price_history (
    price_id       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id     UUID          NOT NULL REFERENCES m1_supply.services,
    price_type     VARCHAR(20)   NOT NULL CHECK (price_type IN ('default','specific')),
    price_source   VARCHAR(20)   NOT NULL DEFAULT 'base'
                       CHECK (price_source IN ('base','bundle_override','volume_tier')),
    amount         NUMERIC(15,2) NOT NULL,
    currency       VARCHAR(3)    NOT NULL DEFAULT 'VND',
    effective_from TIMESTAMPTZ   NOT NULL,
    effective_to   TIMESTAMPTZ,
    created_by     UUID          NOT NULL,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),

    EXCLUDE USING GIST (
        service_id WITH =,
        price_type WITH =,
        tstzrange(effective_from, effective_to) WITH &&
    ) WHERE (price_type = 'specific')
);

CREATE INDEX ON m1_supply.price_history (service_id, effective_from, effective_to);

-- ----------------------------------------------------------------
-- INVENTORY / CAPACITY
-- capacity_used cập nhật bằng atomic UPDATE (D9).
-- allotment_held: phần capacity giữ riêng theo hợp đồng M7.
-- Chưa partition ở Phase 1 (D10), cùng lý do với services — xem index
-- trên airport_code bên dưới làm giải pháp tạm thời.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.inventory (
    slot_id        UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id     UUID          NOT NULL REFERENCES m1_supply.services,
    airport_code   VARCHAR(10)   NOT NULL,
    slot_date      DATE          NOT NULL,
    slot_start     TIMETZ,
    slot_end       TIMETZ,
    capacity_total INT           NOT NULL,
    capacity_used  INT           NOT NULL DEFAULT 0,
    overbook_pct   NUMERIC(5,2)  NOT NULL DEFAULT 0
                       CHECK (overbook_pct BETWEEN 0 AND 20),
    allotment_held INT           NOT NULL DEFAULT 0,
    sync_source    VARCHAR(20)   NOT NULL DEFAULT 'manual'
                       CHECK (sync_source IN ('manual','bulk_upload','api')),
    last_synced_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),

    CONSTRAINT cap_used_non_negative CHECK (capacity_used >= 0),
    UNIQUE (service_id, slot_date, slot_start)
);

CREATE INDEX ON m1_supply.inventory (service_id, slot_date);
CREATE INDEX ON m1_supply.inventory (airport_code);

-- ----------------------------------------------------------------
-- RESERVATIONS (TTL = 15 phút)
-- Auto-release khi order không xác nhận trong thời hạn.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.reservations (
    reservation_id UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    slot_id        UUID        NOT NULL REFERENCES m1_supply.inventory,
    order_ref      UUID        NOT NULL,
    quantity       INT         NOT NULL DEFAULT 1,
    status         VARCHAR(20) NOT NULL DEFAULT 'held'
                       CHECK (status IN ('held','confirmed','released','waitlisted')),
    expires_at     TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '15 minutes'),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m1_supply.reservations (slot_id, status);
CREATE INDEX ON m1_supply.reservations (expires_at) WHERE status = 'held';

-- ----------------------------------------------------------------
-- AUDIT LOG (append-only, partition theo tháng)
-- PK gồm cả created_at vì PostgreSQL yêu cầu partition key nằm trong
-- mọi PK/UNIQUE của bảng partitioned. audit_log không có FK trỏ vào
-- nên đây là bảng duy nhất an toàn để partition ngay từ đầu.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.audit_log (
    log_id       UUID        NOT NULL DEFAULT gen_random_uuid(),
    actor_id     UUID        NOT NULL,
    actor_type   VARCHAR(30) NOT NULL,
    entity_type  VARCHAR(50) NOT NULL,
    entity_id    UUID        NOT NULL,
    action       VARCHAR(50) NOT NULL,
    before_state JSONB,
    after_state  JSONB,
    reason       TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (log_id, created_at)
) PARTITION BY RANGE (created_at);

-- ----------------------------------------------------------------
-- OUTBOX (Transactional Outbox Pattern — D11)
-- INSERT trong cùng transaction với entity change.
-- Relay đọc và publish lên Kafka; không bao giờ DELETE row.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.outbox (
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

CREATE INDEX ON m1_supply.outbox (status, created_at) WHERE status = 'pending';
```

> M2 có bảng `outbox` tương tự trong schema `m2_package` với cấu trúc giống hệt.

### M2 — Schema `m2_package`

```sql
CREATE SCHEMA m2_package;

-- M1 và M2 là hai database riêng biệt (D1) nên extension phải được
-- tạo độc lập ở đây, dùng cho EXCLUDE USING GIST trên revenue_splits.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- ----------------------------------------------------------------
-- PACKAGES (mutable metadata)
-- Package đang active không được sửa trực tiếp.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.packages (
    package_id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name_vi            VARCHAR(255) NOT NULL,
    name_en            VARCHAR(255) NOT NULL,
    description_vi     TEXT,
    description_en     TEXT,
    conditions_vi      TEXT,
    conditions_en      TEXT,
    images             JSONB,
    airport_code       VARCHAR(10)  NOT NULL,
    service_category   VARCHAR(30),
    valid_from         TIMESTAMPTZ,
    valid_to           TIMESTAMPTZ,
    status             VARCHAR(30)  NOT NULL DEFAULT 'draft'
                           CHECK (status IN (
                               'draft','pending_review','active',
                               'paused','replaced','archived'
                           )),
    current_version_id UUID,
    created_by         UUID         NOT NULL,
    approved_by        UUID,
    approved_at        TIMESTAMPTZ,
    opid_ref           VARCHAR(100),
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m2_package.packages (status, airport_code);

-- ----------------------------------------------------------------
-- PACKAGE VERSIONS (immutable)
-- order_items trong M6 tham chiếu version_id, không phải package_id.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.package_versions (
    version_id      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id      UUID        NOT NULL REFERENCES m2_package.packages,
    version         INT         NOT NULL,
    config_snapshot JSONB       NOT NULL,
    published_by    UUID        NOT NULL,
    published_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    replaced_at     TIMESTAMPTZ,

    UNIQUE (package_id, version)
);

-- current_version_id là con trỏ cùng schema nên được FK hoá bình
-- thường để tránh dangling reference (khác với snapshot_id trỏ sang
-- m1_supply, vốn cấm FK theo D1).
ALTER TABLE m2_package.packages
    ADD CONSTRAINT fk_packages_current_version
    FOREIGN KEY (current_version_id) REFERENCES m2_package.package_versions (version_id);

-- ----------------------------------------------------------------
-- PACKAGE ITEMS
-- snapshot_id → m1_supply.service_snapshots (UUID, không FK).
-- Đây là điểm kết nối kỹ thuật quan trọng giữa M1 và M2.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.package_items (
    item_id       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id    UUID        NOT NULL REFERENCES m2_package.package_versions,
    snapshot_id   UUID        NOT NULL,
    service_type  VARCHAR(30) NOT NULL,
    quantity      INT         NOT NULL DEFAULT 1,
    unit          VARCHAR(30),
    display_order INT         NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m2_package.package_items (version_id);
CREATE INDEX ON m2_package.package_items (snapshot_id);

-- ----------------------------------------------------------------
-- PRICES
-- Margin âm cần Finance Manager phê duyệt (app layer).
-- ----------------------------------------------------------------
CREATE TABLE m2_package.prices (
    price_id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id             UUID          NOT NULL REFERENCES m2_package.package_versions,
    base_cost              NUMERIC(15,2) NOT NULL,
    margin_pct             NUMERIC(5,2)  NOT NULL,
    selling_price          NUMERIC(15,2) NOT NULL,
    currency               VARCHAR(3)    NOT NULL DEFAULT 'VND',
    margin_override_reason TEXT,
    approved_by            UUID,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------
-- PRICING RULES
-- commercial_term_id → m7_partner.commercial_terms (UUID, không FK).
-- Phase 1: không cộng dồn — chỉ rule priority cao nhất được áp dụng.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.pricing_rules (
    rule_id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id         UUID          NOT NULL REFERENCES m2_package.package_versions,
    rule_type          VARCHAR(30)   NOT NULL
                           CHECK (rule_type IN (
                               'time_discount','early_bird','last_minute',
                               'flash_sale','partner_tier','bundle_override'
                           )),
    priority           INT           NOT NULL DEFAULT 0,
    discount_type      VARCHAR(10)   NOT NULL CHECK (discount_type IN ('percent','fixed')),
    discount_value     NUMERIC(15,2) NOT NULL,
    min_floor_price    NUMERIC(15,2),
    valid_from         TIMESTAMPTZ   NOT NULL,
    valid_to           TIMESTAMPTZ,
    max_uses           INT,
    uses_count         INT           NOT NULL DEFAULT 0,
    commercial_term_id UUID,
    partner_group_id   UUID,
    conditions         JSONB,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m2_package.pricing_rules (version_id, valid_from, valid_to);

-- ----------------------------------------------------------------
-- REVENUE SPLITS (5 thành phần)
-- CHECK constraint: tổng 3 phần = 100% — bắt buộc tại DB.
-- supplier_id denorm để settlement query không cần join M1.
-- EXCLUDE GIST (cần extension btree_gist ở trên) chặn hai split chồng
-- lấp thời gian cho cùng item_id — cùng cơ chế D6 áp dụng cho
-- price_history, đảm bảo đúng lời hứa "1 split hiệu lực tại 1 thời
-- điểm" của D7.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.revenue_splits (
    split_id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id          UUID         NOT NULL REFERENCES m2_package.package_items,
    version_id       UUID         NOT NULL REFERENCES m2_package.package_versions,
    snapshot_id      UUID         NOT NULL,
    supplier_id      UUID         NOT NULL,
    platform_fee_pct NUMERIC(5,2) NOT NULL CHECK (platform_fee_pct BETWEEN 5 AND 40),
    ace_margin_pct   NUMERIC(5,2) NOT NULL,
    ncc_share_pct    NUMERIC(5,2) NOT NULL,
    vat_rate_ncc     NUMERIC(5,2) NOT NULL DEFAULT 10.0,
    vat_rate_ace     NUMERIC(5,2) NOT NULL DEFAULT 10.0,
    effective_from   TIMESTAMPTZ  NOT NULL,
    effective_to     TIMESTAMPTZ,
    created_by       UUID         NOT NULL,
    approved_by      UUID,

    CONSTRAINT split_sum_100 CHECK (
        ROUND(platform_fee_pct + ace_margin_pct + ncc_share_pct, 2) = 100
    ),
    EXCLUDE USING GIST (
        item_id WITH =,
        tstzrange(effective_from, effective_to) WITH &&
    )
);

CREATE INDEX ON m2_package.revenue_splits (item_id, effective_from);

-- ----------------------------------------------------------------
-- TARGETING RULES
-- conditions: cây AND/OR JSONB, tối đa 5 cấp lồng nhau.
-- deny_wins: nếu allow + deny cùng match → deny được ưu tiên.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.targeting_rules (
    targeting_id UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id   UUID        NOT NULL REFERENCES m2_package.package_versions,
    rule_type    VARCHAR(10) NOT NULL CHECK (rule_type IN ('allow','deny')),
    priority     INT         NOT NULL DEFAULT 0,
    conditions   JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m2_package.targeting_rules (version_id);

-- ----------------------------------------------------------------
-- PROMOTIONS
-- Phase 1: không cộng dồn — rule priority cao nhất thắng.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.promotions (
    promo_id       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id     UUID          NOT NULL REFERENCES m2_package.packages,
    name           VARCHAR(255)  NOT NULL,
    discount_type  VARCHAR(10)   NOT NULL CHECK (discount_type IN ('percent','fixed')),
    discount_value NUMERIC(15,2) NOT NULL,
    valid_from     TIMESTAMPTZ   NOT NULL,
    valid_to       TIMESTAMPTZ   NOT NULL,
    max_uses       INT,
    uses_count     INT           NOT NULL DEFAULT 0,
    partner_scope  UUID[],
    status         VARCHAR(20)   NOT NULL DEFAULT 'active'
                       CHECK (status IN ('draft','active','expired','cancelled')),
    created_by     UUID          NOT NULL,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ON m2_package.promotions (package_id, valid_from, valid_to);

-- ----------------------------------------------------------------
-- A/B TESTS (Phase 1: chia traffic cơ bản theo ACE_UID)
-- ----------------------------------------------------------------
CREATE TABLE m2_package.ab_tests (
    test_id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id        UUID        NOT NULL REFERENCES m2_package.packages,
    variant_a_version UUID        NOT NULL REFERENCES m2_package.package_versions,
    variant_b_version UUID        NOT NULL REFERENCES m2_package.package_versions,
    traffic_split_pct INT         NOT NULL DEFAULT 50
                          CHECK (traffic_split_pct BETWEEN 1 AND 99),
    status            VARCHAR(20) NOT NULL DEFAULT 'running'
                          CHECK (status IN ('running','paused','completed')),
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at          TIMESTAMPTZ
);

-- ----------------------------------------------------------------
-- AUDIT LOG (append-only, partition theo tháng)
-- PK gồm cả created_at, cùng lý do với m1_supply.audit_log ở trên.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.audit_log (
    log_id       UUID        NOT NULL DEFAULT gen_random_uuid(),
    actor_id     UUID        NOT NULL,
    entity_type  VARCHAR(50) NOT NULL,
    entity_id    UUID        NOT NULL,
    action       VARCHAR(50) NOT NULL,
    before_state JSONB,
    after_state  JSONB,
    reason       TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (log_id, created_at)
) PARTITION BY RANGE (created_at);
```

---

## Các quyết định kiến trúc

### D1 — Schema riêng biệt, không shared database

**Quyết định:** M1 dùng schema `m1_supply`, M2 dùng schema `m2_package`. Không có bảng nào chia sẻ giữa hai service. Mọi tham chiếu liên service dùng UUID resolve qua REST API hoặc Kafka event — không bao giờ JOIN xuyên schema.

**Lý do:** Schema chung tạo coupling ẩn — migration bảng trong M1 có thể phá vỡ query của M2 mà không có lỗi compile-time nào báo trước. Schema riêng giữ ranh giới service tường minh và deploy độc lập không cần phối hợp migration.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| M1 và M2 deploy độc lập — migration schema một bên không ảnh hưởng bên kia | Query tổng hợp liên service phải qua API composition hoặc read model riêng |
| Mỗi service chọn index, partition strategy phù hợp với load của nó | Denormalization bắt buộc — `supplier_id` phải lưu trong `revenue_splits` thay vì join M1 |

---

### D2 — Service Snapshot bất biến

**Quyết định:** `services` lưu metadata có thể sửa tự do. Khi ACE phê duyệt, `service_snapshots` lưu bản sao đầy đủ (`snapshot_data JSONB`) đóng băng tại thời điểm đó. Mọi downstream references (`package_items.snapshot_id`, `vouchers.snapshot_id`) trỏ vào `snapshot_id`, không bao giờ trỏ vào `service_id`. Mỗi publish tạo version v+1; snapshot cũ không bao giờ bị sửa hay xoá.

**Lý do:** Nhà cung ứng phải được phép cập nhật mô tả và hình ảnh mà không cần chu kỳ phê duyệt mới. Nhưng người mua và settlement phải thấy chính xác những gì được thoả thuận tại thời điểm mua. Tách bề mặt chỉnh sửa khỏi bản ghi thương mại bất biến đáp ứng cả hai yêu cầu.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Nhà cung ứng sửa metadata sau publish không có tác động hồi tố đến package/voucher | `snapshot_data` JSONB tăng theo mỗi publish; snapshot cũ chỉ archive được sau khi toàn bộ voucher tham chiếu đã settled |
| Tranh chấp quyết toán luôn có câu trả lời dứt khoát | PM phải chủ động publish snapshot mới để thay đổi hiển thị downstream |

---

### D3 — Package Version bất biến

**Quyết định:** `packages` lưu metadata có thể thay đổi. Khi ACE phê duyệt, `package_versions` lưu snapshot cấu hình đầy đủ bao gồm `package_items`, `price`, `pricing_rules`, `revenue_splits` và `targeting_rules`. `order_items` trong M6 tham chiếu `package_version_id`, không bao giờ tham chiếu `package_id`.

**Lý do:** PM cập nhật giá gói hoặc thay thế entitlement sau khi order đã đặt không được ảnh hưởng đến các order đó. Version chain biến "config nào đã được hiển thị cho người mua?" thành tra cứu xác định.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Order đang xử lý hoàn toàn cô lập khỏi thay đổi package | Mọi thay đổi quan trọng cần version mới + chu kỳ phê duyệt lại — tăng overhead |
| PM làm việc an toàn trên draft version song song với version đang active | Clone package phải re-configure revenue split và targeting — không được kế thừa âm thầm |

---

### D4 — Cổng phê duyệt bắt buộc

**Quyết định:** Cả nhà cung ứng lẫn PM đều không thể tự publish vào pipeline thương mại. Mọi chuyển trạng thái thương mại (`supplier → active`, `service → active`, `package → active`) yêu cầu ACE Admin phê duyệt tường minh. Hệ thống chặn kéo service không active vào package; publish package cấu hình chưa đủ; kích hoạt lại package có service thành phần đang paused.

**Lý do:** ACE là operator và chịu trách nhiệm về chất lượng những gì xuất hiện trước người mua. Tự publish không qua gate tạo rủi ro pháp lý và tài chính — nhà cung ứng chưa xác minh hoặc giá sai có thể tiếp cận người mua thực.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| ACE kiểm soát mọi thực thể đến tay người mua | Chu kỳ review tạo độ trễ; ACE Admin trở thành điểm nghẽn nếu queue tăng |
| Audit trail rõ ràng: actor_id, timestamp, lý do, before/after | Kiểm tra tự động (completeness, price sanity) nên được tự động hoá để rút ngắn review thủ công |

---

### D5 — Cascade thay đổi upstream qua Kafka Events

**Quyết định:** Khi M1 phát ra `price.changed`, `service.paused` hoặc `service.deprecated`, M2 consume và tự động đánh dấu package bị ảnh hưởng cần rà soát — nhưng target status khác nhau theo loại sự kiện: `price.changed` → package đang active có entitlement liên quan chuyển ngay về `draft` (gỡ khỏi marketplace, dừng bán); `service.paused`/`service.deprecated` → package liên quan chuyển về `paused`. Cả hai đều là **hard stop tức thời** — khác với việc PM tự tay sửa một package đang active (M2-US-04 AC3), vốn chỉ fork một version draft chạy song song trong khi version đang live vẫn tiếp tục bán bình thường cho tới khi version mới được duyệt. Cascade không dùng cơ chế fork-nền đó vì giá/chi phí nền tảng đã thay đổi — tiếp tục bán theo giá cũ là đúng vấn đề mà D5 phải ngăn chặn.

**Lý do:** Nếu không có cascade tự động, nhà cung ứng tạm dừng dịch vụ hoặc đổi giá lúc 2 giờ sáng sẽ khiến package chứa entitlement đó tiếp tục bán âm thầm theo dữ liệu lỗi thời. Kafka consumer làm hệ thống tự sửa — thay đổi upstream lan truyền trong < 500ms.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Package tự draft/paused khi giá hoặc service thành phần có sự cố — không cần giám sát thủ công | M2 consumer phải idempotent; sự kiện trùng lặp không được double-draft/double-pause |
| PM được thông báo kèm ngữ cảnh cụ thể | Consumer lag trong thời gian sự cố tạo cửa sổ package lỗi thời còn active |

**Luồng khôi phục sau `price.changed` (draft → active trở lại):** package bị cascade về `draft`
không tự phục hồi — phải đi qua đúng pipeline phê duyệt như một chỉnh sửa package bình thường:

```
Package → draft (do price.changed)
     │
     ▼
PM cập nhật margin/selling_price theo giá M1 mới (M1-US-05 / M2-US-05)
Finance Manager cấu hình lại revenue_split cho version mới nếu cần (D7 —
package_items của version mới là row mới, không kế thừa revenue_splits cũ)
     │
     ▼
Submit → status = pending_review ("Review" theo M2-US-04 AC1/AC2)
     │
     ▼
ACE Admin review & approve (cổng phê duyệt D4 — kiểm tra price sanity,
revenue split sum=100%, platform_fee 5–40%)
     │
     ▼
package_versions v+1 tạo (bất biến — D3): prices/package_items/
revenue_splits mới. packages.current_version_id → v+1, status = active
event: package.published → Kafka (M6 catalog nhận version mới, package
bán lại được)
     │
     ▼
(7 ngày kể từ thời điểm thay đổi — M2-US-04 AC4) ACE Admin có thể rollback
current_version_id về version trước đó nếu version mới sai
```

Trong suốt luồng này, order đã đặt dưới version cũ (trước khi giá đổi) hoàn toàn không bị ảnh
hưởng — `order_items` đã khoá `package_version_id` + `price_snapshot` + `revenue_split_version_id`
tại thời điểm `order.confirmed` (D3, D7).

---

### D6 — Price History append-only với EXCLUDE USING GIST

**Quyết định:** `price_history` là bảng append-only — không UPDATE, không DELETE. Khi giá thay đổi: INSERT row mới với `effective_from` mới, set `effective_to` của row cũ. Constraint `EXCLUDE USING GIST` dùng `tstzrange` chặn hai specific price chồng lấp thời gian cho cùng service.

**Lý do:** Giá là bằng chứng pháp lý trong tranh chấp thanh toán. Bảng giá dùng UPDATE phá huỷ khả năng chứng minh giá nào đang hiệu lực tại một thời điểm cụ thể trong quá khứ.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Tái tạo chính xác giá tại bất kỳ thời điểm nào | Query "giá hiện tại" phức tạp hơn: filter theo `effective_from <= now() AND (effective_to IS NULL OR effective_to > now())` |
| Rollback = INSERT row mới với giá trị cũ; không có thao tác phá huỷ | GIST index thêm ~5ms vào price INSERT — chấp nhận được vì tần suất thay đổi giá thấp |

---

### D7 — Revenue Split có versioning và CHECK constraint

**Quyết định:** `revenue_splits` có `effective_from/effective_to`. Tại thời điểm xác nhận order, M6 khoá ID row đang hiệu lực vào `order_items.revenue_split_version_id`. Constraint `ROUND(platform_fee_pct + ace_margin_pct + ncc_share_pct, 2) = 100` được enforce tại DB level. Tương tự D6, một `EXCLUDE USING GIST` trên `(item_id, tstzrange(effective_from, effective_to))` chặn hai split chồng lấp thời gian cho cùng `item_id` ngay tại INSERT.

**Lý do:** Điều khoản tái đàm phán giữa chừng không được hồi tố thay đổi payout cho giao dịch đã hoàn thành. Constraint tổng = 100% là lưới an toàn cuối cùng — split không hợp lệ bị bắt tại INSERT, không phải lúc settlement 30 ngày sau.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Settlement tại T+n dùng đúng split thoả thuận tại T=0 | `ROUND` trong constraint phải khớp với `shopspring/decimal` trong Go — divergence gây lỗi 500 khi INSERT hợp lệ |
| DB constraint bắt lỗi sớm nhất có thể; `EXCLUDE` chặn overlap split cùng lúc INSERT, không cần lock ứng dụng | Split `effective_from` phải phối hợp với contract activation trong M7; `EXCLUDE` cần extension `btree_gist` được tạo trong database `m2_package` |

---

### D8 — JSONB cho dữ liệu linh hoạt

**Quyết định:** Ba trường hợp dùng JSONB: `service_snapshots.snapshot_data` (metadata service đóng băng), `package_versions.config_snapshot` (cấu hình package), và `targeting_rules.conditions` (cây logic AND/OR). Tất cả các trường còn lại là typed column.

**Lý do:** Snapshot cần lưu cấu trúc có thể thay đổi theo thời gian (service type mới, field mới) mà không cần ALTER TABLE. Targeting conditions cần biểu diễn cây logic có độ sâu biến đổi — không thể normalize thành bảng quan hệ tự nhiên.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Thêm field mới vào snapshot không cần migration | JSONB không có type safety ở DB — application layer phải validate schema khi đọc |
| Targeting conditions biểu diễn tự nhiên cây AND/OR | Query trực tiếp vào JSONB chậm hơn typed column; cần GIN index nếu query thường xuyên |

---

### D9 — Two-phase inventory với atomic UPDATE

**Quyết định:** Giai đoạn 1: khi `order.confirmed`, tạo `reservations` row và atomic UPDATE `capacity_used += qty`. Giai đoạn 2: khi `voucher.redeemed`, confirm reservation — không trừ capacity thêm lần nữa. Nếu hủy/hết hạn: atomic UPDATE `capacity_used -= qty`, status = 'released'.

**Lý do:** SELECT rồi UPDATE tạo race condition khi nhiều checkout đồng thời — dễ overbooking. Trừ inventory hai lần (order + redeem) dẫn đến double-deduction. Atomic UPDATE và two-phase giải quyết cả hai.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Không overbooking do race condition | Cần cron job dọn reservation hết hạn (index trên `expires_at WHERE status='held'`) |
| Không double-deduction — voucher.redeemed chỉ confirm, không trừ thêm | `capacity_used` bị lock trong transaction — cần timeout ngắn để tránh contention |

---

### D10 — Partitioning theo `airport_code` (LIST) và `created_at` (RANGE)

**Quyết định:** Chỉ `audit_log` (cả hai schema) dùng `PARTITION BY RANGE (created_at)` theo tháng ngay từ đầu — không có bảng nào FK vào `audit_log` nên PK có thể mở rộng thành `(log_id, created_at)` để thoả yêu cầu PostgreSQL (partition key phải nằm trong mọi PK/UNIQUE). `services`, `inventory` và `price_history` **chưa** partition ở Phase 1: `PARTITION BY LIST (airport_code)` sẽ buộc PK của `services`/`inventory` phải mở rộng thành `(airport_code, ...)`, kéo theo mọi FK trỏ vào `service_id` (từ `service_snapshots`, `price_history`, `inventory`, `reservations`) phải đổi thành composite key; `price_history` còn phức tạp hơn vì `EXCLUDE USING GIST` hiện định nghĩa trên `(service_id, price_type, tstzrange)`, không phải trên `created_at`, nên partition theo `created_at` sẽ không tương thích với constraint đó nếu không thiết kế lại. Việc partition ba bảng này được để lại cho một migration riêng khi thực sự vượt ngưỡng 10M rows; trong lúc chờ, `CREATE INDEX ... (airport_code)` trên `services` và `inventory` đóng vai trò giải pháp tạm để giữ mục tiêu "chỉ scan 1 sân bay".

**Lý do:** Phần lớn query vận hành đều có filter `WHERE airport_code = X` — partition loại bỏ toàn bộ airport khác. Range partition theo tháng cho phép DROP partition cũ trong milliseconds thay vì DELETE hàng triệu rows. Nhưng partition ngay từ Phase 1 khi row count còn nhỏ (<1M) tạo rủi ro DDL không hợp lệ hoặc phải thiết kế lại constraint sớm — trong khi lợi ích (partition pruning) chưa cần thiết ở quy mô này.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| `audit_log` DROP partition cũ nhanh hơn DELETE ngay từ Phase 1 | `services`/`inventory`/`price_history` chưa có partition pruning ở Phase 1 — bù bằng index thường trên `airport_code`, chấp nhận được vì row count còn nhỏ (<1M) |
| Không phải thiết kế lại composite PK/FK hay `EXCLUDE` constraint ngay bây giờ | Cần script tạo partition + migration composite key/FK cho `services`/`inventory`/`price_history` khi vượt ngưỡng 10M rows — phải lên kế hoạch trước, không phải việc bật công tắc đơn giản |

---

### D11 — Transactional Outbox Pattern cho Kafka publishing

**Quyết định:** Mỗi service (M1, M2) có bảng `outbox` trong schema của nó. Khi một use case cần phát sinh Kafka event, thay vì gọi Kafka trực tiếp, service INSERT một row vào `outbox` trong cùng DB transaction với entity change. Một background goroutine (Outbox Relay) poll bảng `outbox` mỗi 500ms, publish lên Kafka, rồi mark `status = 'published'`. Relay dùng `SELECT ... FOR UPDATE SKIP LOCKED` để nhiều instance chạy song song không conflict. Consumer phía nhận phải idempotent — dùng bảng `processed_events` với `ON CONFLICT DO NOTHING` để bỏ qua duplicate.

**Lý do:** Không có distributed transaction giữa Aurora PostgreSQL và Amazon MSK. Cách publish Kafka sau khi commit DB tạo khoảng thời gian crash có thể mất message vĩnh viễn — M2 không nhận được `service.paused`/`price.changed` nên cascade không xảy ra, package chứa entitlement đã lỗi thời vẫn tiếp tục bán, hệ thống silent inconsistent mà không có cảnh báo nào.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Đảm bảo at-least-once delivery — event không bao giờ mất dù crash tại bất kỳ điểm nào | Relay tạo thêm ~500ms latency so với publish trực tiếp; chấp nhận được cho ACE Phase 1 |
| Entity state và event luôn nhất quán — không có trạng thái "DB đã thay đổi nhưng event chưa gửi" | Consumer bắt buộc phải idempotent — thêm bảng `processed_events` và kiểm tra trước khi xử lý |
| Không cần distributed transaction hay saga orchestrator phức tạp | `outbox` tăng trưởng theo thời gian; cần job dọn row `published` cũ hơn 7 ngày |
| Nhiều relay instance chạy song song an toàn nhờ `SKIP LOCKED` | Nếu Kafka down kéo dài, `outbox` tích luỹ; cần alert khi `retry_count > 3` |

---

## Hệ quả

| Thách thức | Đảm bảo | Lưu ý |
|------------|---------|-------|
| **Tính bất biến** | Snapshot chain (service_snapshot → package_version → order_item) đảm bảo chỉnh sửa upstream không có tác động hồi tố. Price history append-only với GIST exclusion đảm bảo giá tại bất kỳ thời điểm nào đều truy vết được. | Snapshot và price_history tăng trưởng không giới hạn. Cần archival job: snapshot archive sau khi toàn bộ voucher tham chiếu đã expired/settled; price_history cũ rotate sang S3 sau retention window. |
| **Thương mại hoá có kiểm soát** | Không có service hay package nào vào pipeline thương mại mà không qua ACE approval. Cascade Kafka tự paused package khi service thành phần có sự cố trong < 500ms. | ACE Admin review queue là rủi ro điểm nghẽn. Pre-validation checks (completeness, price sanity) nên tự động hoá trong submit workflow. |
| **Tính toàn vẹn tài chính** | Revenue split CHECK constraint tổng = 100% tại DB. Split version khoá tại checkout. Schema riêng biệt giữ M1/M2 deploy độc lập. | Split `effective_from` phải phối hợp với contract activation trong M7. Dangling UUID references (M2 trỏ snapshot đã archive trong M1) cần audit job định kỳ kiểm tra. |
| **Hiệu năng** | Atomic inventory UPDATE chặn race condition overbooking. Partitioning giữ query performance khi > 10M rows mà không rewrite query. | Partition LIST cần script tự động tạo partition mới khi mở sân bay. Index nên monitor bằng `pg_stat_user_indexes` sau khi có traffic thực — không thêm index speculative. |
| **Tính tin cậy message** | Outbox Pattern đảm bảo at-least-once delivery — không có event nào bị mất dù crash tại bất kỳ điểm nào trong luồng xử lý. | Consumer phải idempotent. Cần alert khi `outbox.retry_count > 3` (Kafka có thể down). Job dọn row `published` cũ hơn 7 ngày để tránh bảng phình to. |

**Hệ quả bổ sung:**

- **M1 ship trước M2:** M2 chỉ cần API và Kafka events của M1 ổn định. Contract cần freeze trước khi M1 ship: `GET /v1/services/{id}/snapshots` response shape (PM pull khi xây dựng gói — không cần event) và payload Kafka của các sự kiện cascade (`service.paused`, `price.changed`).
- **`opid_ref` nullable trên mọi bảng identity:** Phase 2 identity merge (ACE_UID → OPID) chỉ cần data backfill, không cần ALTER TABLE.
- **Denormalization có chủ đích:** `service_type` trong `package_items` và `supplier_id` trong `revenue_splits` được lưu lại để settlement và analytics query không cần API call về M1.
- **Phase 2 targeting readiness:** `targeting_rules.conditions` đã hỗ trợ field `flight_class`, `loyalty_tier`, `opid_ref` trong JSONB từ Phase 1 — không cần migration khi OPID integration đi vào hoạt động.
