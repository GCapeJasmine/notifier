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
5. **Tính tin cậy của message publishing** — sự kiện Kafka (`service.approved`, `price.changed`, `package.published`) phải được đảm bảo gửi đúng một lần; crash giữa chừng không được làm mất event hoặc để hệ thống rơi vào trạng thái không nhất quán giữa DB và message broker.

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
  draft ──▶ pending_approval ──▶ active ──▶ paused
                                      └──▶ replaced ──▶ archived

  Cascade khi upstream thay đổi (qua Kafka):
  service price changes  ──▶  package liên quan → pending_review (tạm dừng bán)
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

**Vấn đề:** M1 và M2 phát sinh Kafka events tại các điểm chuyển trạng thái quan trọng (`service.approved`, `price.changed`, `package.published`). Cách triển khai naive — commit DB trước, publish Kafka sau — tạo khoảng thời gian nguy hiểm:

```
  UPDATE services SET status='active'
  tx.Commit()                        ← DB đã ghi, không rollback được
  kafka.Publish("service.approved")  ← crash tại đây → message mất vĩnh viễn

  Hệ quả: M2 không biết service đã approved
          → package không thể kéo snapshot mới
          → PM tưởng hệ thống bị treo, tự thao tác lại → duplicate state
```

Publish Kafka trước, commit DB sau cũng không giải quyết được — nếu DB commit fail, event đã đến consumer và gây side-effect không có bản ghi tương ứng.

**Giải pháp — Transactional Outbox Pattern:**

```
  Trong cùng 1 DB transaction:
  ┌──────────────────────────────────────────────────────────┐
  │  BEGIN                                                   │
  │  UPDATE services SET status = 'active'                   │
  │  INSERT INTO outbox (event_type, payload)                │
  │         VALUES ('service.approved', '{...}')             │
  │  COMMIT  ← atomic: cả hai cùng thành công hoặc cùng fail │
  └──────────────────────────────────────────────────────────┘
           │
           │  (DB đã commit, outbox row tồn tại)
           ▼
  ┌──────────────────────────────────────────────────────────┐
  │  Outbox Dispatcher — 2 phương án, xem D11:               │
  │  A) Polling Relay: goroutine SELECT ... FOR UPDATE       │
  │     SKIP LOCKED mỗi 500ms → kafka.Publish → mark         │
  │     'published' (chọn cho Phase 1)                       │
  │  B) Debezium CDC: đọc WAL qua logical replication,       │
  │     không polling, publish gần real-time (nâng cấp sau)  │
  └──────────────────────────────────────────────────────────┘
           │
           ▼  (cả hai phương án chỉ đảm bảo at-least-once —
               có thể publish trùng)
  ┌──────────────────────────────────────────────────────────┐
  │  Consumer phải idempotent                                │
  │  INSERT INTO processed_events (event_id)                 │
  │  ON CONFLICT DO NOTHING  ← skip nếu đã xử lý             │
  └──────────────────────────────────────────────────────────┘
```

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
  │  event: service.approved → Kafka                             │
  └──────────────────────┬───────────────────────────────────────┘
         │ snapshot_id sẵn sàng cho M2
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
  ──▶ M2 consumer: package chứa snapshot_id → pending_review, tạm dừng bán

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
-- Partition theo airport_code khi > 10M rows (D10).
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
) PARTITION BY LIST (airport_code);

CREATE INDEX ON m1_supply.services (supplier_id, status);
CREATE INDEX ON m1_supply.services (service_type);

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

-- ----------------------------------------------------------------
-- PRICE HISTORY (append-only)
-- Khi giá thay đổi: INSERT row mới, KHÔNG UPDATE row cũ.
-- EXCLUDE GIST chặn hai specific price chồng lấp thời gian.
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
                       CHECK (status IN ('held','confirmed','released')),
    expires_at     TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '15 minutes'),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON m1_supply.reservations (slot_id, status);
CREATE INDEX ON m1_supply.reservations (expires_at) WHERE status = 'held';

-- ----------------------------------------------------------------
-- AUDIT LOG (append-only, partition theo tháng)
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.audit_log (
    log_id       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id     UUID        NOT NULL,
    actor_type   VARCHAR(30) NOT NULL,
    entity_type  VARCHAR(50) NOT NULL,
    entity_id    UUID        NOT NULL,
    action       VARCHAR(50) NOT NULL,
    before_state JSONB,
    after_state  JSONB,
    reason       TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
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
                               'draft','pending_approval','active',
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
-- ----------------------------------------------------------------
CREATE TABLE m2_package.audit_log (
    log_id       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id     UUID        NOT NULL,
    entity_type  VARCHAR(50) NOT NULL,
    entity_id    UUID        NOT NULL,
    action       VARCHAR(50) NOT NULL,
    before_state JSONB,
    after_state  JSONB,
    reason       TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
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

**Quyết định:** Khi M1 phát ra `price.changed`, `service.paused` hoặc `service.deprecated`, M2 consume và tự động đánh dấu package bị ảnh hưởng cần rà soát. Package đang active có service thành phần bị paused tự động chuyển về `paused` — không thể tiếp tục bán cho đến khi PM review.

**Lý do:** Nếu không có cascade tự động, nhà cung ứng tạm dừng dịch vụ lúc 2 giờ sáng sẽ khiến package chứa entitlement đó tiếp tục bán âm thầm. Kafka consumer làm hệ thống tự sửa — thay đổi upstream lan truyền trong < 500ms.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Package tự paused khi service thành phần có sự cố — không cần giám sát thủ công | M2 consumer phải idempotent; sự kiện trùng lặp không được double-pause |
| PM được thông báo kèm ngữ cảnh cụ thể | Consumer lag trong thời gian sự cố tạo cửa sổ package lỗi thời còn active |

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

**Quyết định:** `revenue_splits` có `effective_from/effective_to`. Tại thời điểm xác nhận order, M6 khoá ID row đang hiệu lực vào `order_items.revenue_split_version_id`. Constraint `ROUND(platform_fee_pct + ace_margin_pct + ncc_share_pct, 2) = 100` được enforce tại DB level.

**Lý do:** Điều khoản tái đàm phán giữa chừng không được hồi tố thay đổi payout cho giao dịch đã hoàn thành. Constraint tổng = 100% là lưới an toàn cuối cùng — split không hợp lệ bị bắt tại INSERT, không phải lúc settlement 30 ngày sau.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Settlement tại T+n dùng đúng split thoả thuận tại T=0 | `ROUND` trong constraint phải khớp với `shopspring/decimal` trong Go — divergence gây lỗi 500 khi INSERT hợp lệ |
| DB constraint bắt lỗi sớm nhất có thể | Split `effective_from` phải phối hợp với contract activation trong M7 |

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

**Quyết định:** `services` và `inventory` dùng `PARTITION BY LIST (airport_code)` khi > 10M rows. `audit_log` và `price_history` dùng `PARTITION BY RANGE (created_at)` theo tháng.

**Lý do:** Phần lớn query vận hành đều có filter `WHERE airport_code = X` — partition loại bỏ toàn bộ airport khác. Range partition theo tháng cho phép DROP partition cũ trong milliseconds thay vì DELETE hàng triệu rows.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Query theo airport chỉ scan đúng 1 partition | Cần script tạo partition tự động khi ACE mở sân bay mới — quên tạo → INSERT fail |
| DROP partition cũ nhanh hơn DELETE | Phase 1 chưa cần partition (< 1M rows); kích hoạt khi thực sự vượt ngưỡng 10M |

---

### D11 — Transactional Outbox Pattern cho Kafka publishing

**Quyết định:** Mỗi service (M1, M2) có bảng `outbox` trong schema của nó. Khi một use case cần phát sinh Kafka event, thay vì gọi Kafka trực tiếp, service INSERT một row vào `outbox` trong cùng DB transaction với entity change — atomic với entity change, không có khoảng hở giữa DB commit và event publish. Có hai phương án cho phần "đọc outbox và đẩy lên Kafka" (outbox dispatcher); ACE chọn **Phương án A (Polling Relay)** cho Phase 1 và cân nhắc **Phương án B (Debezium CDC)** khi quy mô event tăng.

**Phương án A — Polling Relay (background goroutine tự viết):**

```
  Một background goroutine (Outbox Relay) poll bảng outbox mỗi 500ms:
  SELECT ... FROM outbox WHERE status='pending'
  FOR UPDATE SKIP LOCKED     ← nhiều instance không conflict
  kafka.Publish(event)
  UPDATE outbox SET status='published'
```

Relay chạy trong cùng codebase Go của service, dùng `status` (`pending`/`published`/`failed`) và `retry_count` để theo dõi tiến độ publish.

**Phương án B — Debezium CDC (logical replication, không polling):**

```
  Debezium Postgres Connector (chạy trong Kafka Connect)
  đọc logical replication slot (WAL) của Aurora — KHÔNG query bảng outbox
  Outbox Event Router SMT:
    - route theo aggregate_type → đúng Kafka topic
    - payload JSONB → Kafka message value
  Publish lên Kafka ngay khi transaction commit xuất hiện trong WAL
```

Với phương án này, bảng `outbox` không cần `status`/`retry_count` vì không relay nào cập nhật trạng thái — WAL đã capture sự kiện tại thời điểm commit bất kể row sau đó còn hay đã bị dọn.

Ở cả hai phương án, consumer phía nhận vẫn phải idempotent — dùng bảng `processed_events` với `ON CONFLICT DO NOTHING` để bỏ qua duplicate, vì cả polling relay lẫn Debezium đều chỉ đảm bảo at-least-once, không phải exactly-once.

**Lý do:** Không có distributed transaction giữa Aurora PostgreSQL và Amazon MSK. Cách publish Kafka sau khi commit DB tạo khoảng thời gian crash có thể mất message vĩnh viễn — M2 không nhận được `service.approved` nên không thể kéo snapshot mới, cascade không xảy ra, hệ thống silent inconsistent mà không có cảnh báo nào. Cả hai phương án đều đóng khoảng hở này bằng cách tách "ghi event" (trong transaction) khỏi "gửi event" (ngoài transaction); khác biệt chỉ ở cách đọc outbox.

**So sánh Polling Relay vs Debezium CDC:**

| Tiêu chí | Phương án A — Polling Relay | Phương án B — Debezium CDC |
|---|---|---|
| Latency | ~500ms (chu kỳ poll) dù hệ thống rảnh | Gần real-time (sub-100ms) — đọc WAL ngay khi commit |
| Tải lên DB | SELECT lặp lại mỗi 500ms + `FOR UPDATE SKIP LOCKED` trên mọi instance | Không query bảng — tail WAL stream Postgres vốn đã ghi cho replication |
| Schema `outbox` | Cần `status`, `retry_count` để theo dõi state machine pending→published→failed | Không cần `status`/`retry_count` — schema gọn hơn |
| Hạ tầng bổ sung | Không — chạy trong process Go hiện có | Cần Kafka Connect cluster + Debezium connector (thành phần vận hành mới) |
| Yêu cầu DB | Không đổi | Phải bật `wal_level=logical` trên Aurora — tăng WAL retention, ảnh hưởng storage/failover |
| Rủi ro vận hành | Relay crash → outbox tích luỹ, dễ phát hiện qua `retry_count`/queue depth | Connector down lâu → replication slot bị giữ, WAL phình to không giới hạn cho tới khi connector chạy lại hoặc slot bị drop |
| Độ phức tạp triển khai | Thấp — team đã quen Go, không thêm dependency hạ tầng | Cao hơn — cần kỹ năng vận hành Kafka Connect/Debezium, cấu hình Outbox Event Router SMT |
| Phù hợp | Phase 1 (một vài sân bay, event throughput thấp–vừa) | Khi throughput lớn hoặc latency 500ms không chấp nhận được (Phase 3–4) |

**Đánh đổi (Phương án A — lựa chọn Phase 1):**

| Lợi ích | Chi phí |
|---------|---------|
| Đảm bảo at-least-once delivery — event không bao giờ mất dù crash tại bất kỳ điểm nào | Relay tạo thêm ~500ms latency so với publish trực tiếp; chấp nhận được cho ACE Phase 1 |
| Entity state và event luôn nhất quán — không có trạng thái "DB đã thay đổi nhưng event chưa gửi" | Consumer bắt buộc phải idempotent — thêm bảng `processed_events` và kiểm tra trước khi xử lý |
| Không cần distributed transaction, saga orchestrator, hay hạ tầng Kafka Connect/Debezium phức tạp | `outbox` tăng trưởng theo thời gian; cần job dọn row `published` cũ hơn 7 ngày |
| Nhiều relay instance chạy song song an toàn nhờ `SKIP LOCKED` | Nếu Kafka down kéo dài, `outbox` tích luỹ; cần alert khi `retry_count > 3` |

**Đường nâng cấp:** Nếu Phase 3–4 cần latency thấp hơn 500ms hoặc polling tạo tải DB đáng kể, chuyển sang Debezium CDC là migration tương thích ngược — bảng `outbox` giữ nguyên PK/columns cốt lõi (`aggregate_type`, `aggregate_id`, `event_type`, `payload`), chỉ cần bổ sung `wal_level=logical`, triển khai Kafka Connect, và có thể loại bỏ `status`/`retry_count` sau khi tắt relay cũ.

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

- **M1 ship trước M2:** M2 chỉ cần API và Kafka events của M1 ổn định. Contract cần freeze trước khi M1 ship: `GET /v1/services/{id}/snapshots` response shape và `service.approved` Kafka payload.
- **`opid_ref` nullable trên mọi bảng identity:** Phase 2 identity merge (ACE_UID → OPID) chỉ cần data backfill, không cần ALTER TABLE.
- **Denormalization có chủ đích:** `service_type` trong `package_items` và `supplier_id` trong `revenue_splits` được lưu lại để settlement và analytics query không cần API call về M1.
- **Phase 2 targeting readiness:** `targeting_rules.conditions` đã hỗ trợ field `flight_class`, `loyalty_tier`, `opid_ref` trong JSONB từ Phase 1 — không cần migration khi OPID integration đi vào hoạt động.
