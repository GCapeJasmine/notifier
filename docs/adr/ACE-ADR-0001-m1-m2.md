# ADR-0001: Kiến trúc và Thiết kế Database M1 Supply Service & M2 Package Service

---

## Bối cảnh

M1 và M2 tạo thành lớp cung ứng và lớp sản phẩm của nền tảng ACE. Hai module này cùng trả lời hai câu hỏi:

- **M1:** Dịch vụ nào đang tồn tại, ai cung cấp, giá bao nhiêu, còn bao nhiêu năng lực phục vụ?
- **M2:** Các dịch vụ đó được đóng gói thành sản phẩm bán được như thế nào, định giá ra sao, và hiển thị cho đúng người mua nào?

5 thách thức cốt lõi chi phối mọi quyết định kiến trúc và thiết kế database trong tài liệu này:

1. **Tính bất biến** — giá, định nghĩa dịch vụ và cấu hình gói được thoả thuận tại thời điểm giao dịch không bao giờ được thay đổi hồi tố, dù nhà cung ứng có chỉnh sửa metadata hay giá thay đổi về sau.
2. **Thương mại hoá có kiểm soát** — không có dịch vụ hay gói nào đến tay người mua mà không qua cổng phê duyệt thuộc ACE; nhà cung ứng và Product Manager không thể tự publish thẳng ra marketplace.
3. **Tính toàn vẹn tài chính** — revenue split giữa ACE và từng nhà cung ứng phải được khoá tại thời điểm bán và có thể truy vết về đúng điều khoản thương mại hiệu lực tại thời điểm đó.
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
  service price changes  ──▶  package liên quan → draft + alert PM (tạm dừng bán, cần PM submit lại package)
  service paused/depr.   ──▶  package liên quan → paused + alert PM
```

Quyết định: **D4**, **D5**

---

### Thách thức 3 — Tính toàn vẹn tài chính và tham chiếu liên service

**Vấn đề:** Một gói bundling entitlement từ nhiều nhà cung ứng — mỗi entitlement có platform fee, ACE margin và VAT riêng. Điều khoản thương mại thay đổi giữa chừng. Nếu revenue split không được khoá tại thời điểm giao dịch, settlement sẽ bị lệch.

**Giải pháp — Revenue split versioned + UUID cross-reference:**

```
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
  Indexing (Phase 1 — không bảng nào partition):
  audit_log ──▶ B-tree index trên created_at
               cleanup bằng DELETE theo batch (không DROP partition)
  services, inventory ──▶ B-tree index trên airport_code
               đủ cho query "chỉ scan 1 sân bay" ở quy mô Phase 1
  price_history ──▶ EXCLUDE GIST sẵn có (service_id, price_type,
               tstzrange) đã phục vụ luôn mục đích lọc theo service

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

**Giải pháp — Transactional Outbox Pattern qua Debezium CDC:**

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
           │  (DB đã commit, outbox row tồn tại trong WAL)
           ▼
  ┌──────────────────────────────────────────────────────────┐
  │  Debezium PostgreSQL Connector (Kafka Connect)           │
  │  Đọc trực tiếp write-ahead log (WAL) qua logical         │
  │  replication slot (wal_level=logical) — KHÔNG polling,   │
  │  event-driven theo LSN mỗi khi WAL advance               │
  │                                                          │
  │  Outbox Event Router SMT "bóc" row → Kafka message:      │
  │  topic = event_type, key = aggregate_id, value = payload │
  │  Offset (LSN) do Kafka Connect tự lưu — không cần cột    │
  │  status/retry_count/published_at ở tầng DB               │
  └──────────────────────────────────────────────────────────┘
           │
           ▼  (at-least-once — connector có thể replay từ LSN
               cuối cùng đã commit nếu restart)
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
  ──▶ M2 consumer: package chứa snapshot_id → draft, tạm dừng bán, alert PM

  event: service.paused (Kafka)
  ──▶ M2 consumer: package chứa snapshot_id → paused, alert PM


 ┌────────────────────────────────────────────────────────────────────────────┐
 │                              ERD                                           │
 └────────────────────────────────────────────────────────────────────────────┘

  M1 (schema: m1_supply)
  suppliers ──▶ UUID user_management.user_role_assignments.tenant_id
             (tenant_type='supplier' — identity/role của user thuộc nhà
             cung ứng do User Management Service sở hữu, xem ADR-0004)
  suppliers ──< services ──────< service_snapshots  (immutable)
                         ──────< price_history       (append-only)
                         ──────< inventory ──────────< reservations (TTL 15p)
  audit_log
  outbox  (Debezium CDC → Kafka)

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
-- SUPPLIER USERS — KHÔNG còn bảng riêng ở đây.
-- Identity, role (admin/operator/viewer) và session của user thuộc
-- nhà cung ứng nay do User Management Service sở hữu hoàn toàn
-- (xem ACE-ADR-0004-user-management.md — user_role_assignments với
-- tenant_type='supplier', tenant_id=suppliers.supplier_id, KHÔNG FK
-- xuyên schema, cùng nguyên tắc D1). M1 chỉ cần supplier_id để trỏ
-- sang, không lưu trùng danh tính/role ở đây.
-- ----------------------------------------------------------------

-- ----------------------------------------------------------------
-- SERVICES (mutable)
-- Không được tham chiếu trực tiếp bởi package/voucher.
-- KHÔNG partition (D10) — PARTITION BY LIST (airport_code) đòi hỏi
-- airport_code nằm trong PK của chính bảng này. Index trên
-- airport_code bên dưới là thiết kế Phase 1.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.services (
    service_id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id         UUID         NOT NULL,
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
    service_id     UUID        NOT NULL,
    version        INT         NOT NULL,
    snapshot_data  JSONB       NOT NULL,
    published_by   UUID        NOT NULL,
    published_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (service_id, version)
);

-- current_snapshot_id: con trỏ cùng schema
-- tồn tại ở application layer trước khi cập nhật.

-- ----------------------------------------------------------------
-- PRICE HISTORY (append-only)
-- Khi giá thay đổi: INSERT row mới, KHÔNG UPDATE row cũ.
-- EXCLUDE GIST (cần extension btree_gist ở trên) chặn hai specific
-- price chồng lấp thời gian. KHÔNG partition (D10): RANGE BY created_at
-- sẽ xung đột với EXCLUDE constraint hiện tại (không định nghĩa trên
-- created_at).
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.price_history (
    price_id       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id     UUID          NOT NULL,
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
-- KHÔNG partition (D10), cùng lý do với services — index trên
-- airport_code bên dưới là thiết kế Phase 1.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.inventory (
    slot_id        UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id     UUID          NOT NULL,
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
    slot_id        UUID        NOT NULL,
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
-- AUDIT LOG (append-only)
-- KHÔNG partition ở Phase 1 (D10) — index trên created_at đủ cho
-- truy vấn theo khoảng thời gian; cleanup dùng DELETE theo batch
-- thay vì DROP partition.
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
);

CREATE INDEX ON m1_supply.audit_log (created_at);

-- ----------------------------------------------------------------
-- OUTBOX (Transactional Outbox Pattern — D11, CDC qua Debezium)
-- INSERT trong cùng transaction với entity change. KHÔNG có cột
-- status/retry_count/published_at — Debezium đọc trực tiếp WAL qua
-- logical replication, không polling nên không cần đánh dấu
-- "pending/published" ở tầng DB; offset (LSN) do Kafka Connect tự
-- quản lý. Row giữ lại để phục vụ replay/debug, dọn định kỳ theo
-- created_at (không phải theo status) — không bao giờ DELETE ngay.
-- ----------------------------------------------------------------
CREATE TABLE m1_supply.outbox (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(50)  NOT NULL,
    aggregate_id   UUID         NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB        NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON m1_supply.outbox (created_at);
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
    package_id      UUID        NOT NULL,
    version         INT         NOT NULL,
    config_snapshot JSONB       NOT NULL,
    published_by    UUID        NOT NULL,
    published_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    replaced_at     TIMESTAMPTZ,

    UNIQUE (package_id, version)
);

-- current_version_id: con trỏ cùng schema
-- tồn tại ở application layer trước khi cập nhật.

-- ----------------------------------------------------------------
-- PACKAGE ITEMS
-- snapshot_id → m1_supply.service_snapshots (UUID).
-- Đây là điểm kết nối kỹ thuật quan trọng giữa M1 và M2.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.package_items (
    item_id       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id    UUID        NOT NULL,
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
    version_id             UUID          NOT NULL,
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
-- commercial_term_id → m7_partner.commercial_terms (UUID).
-- Phase 1: không cộng dồn — chỉ rule priority cao nhất được áp dụng.
-- ----------------------------------------------------------------
CREATE TABLE m2_package.pricing_rules (
    rule_id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id         UUID          NOT NULL,
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
    item_id          UUID         NOT NULL,
    version_id       UUID         NOT NULL,
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
    version_id   UUID        NOT NULL,
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
    package_id     UUID          NOT NULL,
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
    package_id        UUID        NOT NULL,
    variant_a_version UUID        NOT NULL,
    variant_b_version UUID        NOT NULL,
    traffic_split_pct INT         NOT NULL DEFAULT 50
                          CHECK (traffic_split_pct BETWEEN 1 AND 99),
    status            VARCHAR(20) NOT NULL DEFAULT 'running'
                          CHECK (status IN ('running','paused','completed')),
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at          TIMESTAMPTZ
);

-- ----------------------------------------------------------------
-- AUDIT LOG (append-only)
-- KHÔNG partition, cùng lý do với m1_supply.audit_log ở trên.
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
);

CREATE INDEX ON m2_package.audit_log (created_at);
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

> Nguyên tắc "UUID, không FK" ở đây áp dụng cho tham chiếu **xuyên schema**.
> này cho cả tham chiếu **trong cùng schema** (M1 nội bộ, M2 nội bộ).

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

**Quyết định:** Khi M1 phát ra `price.changed`, `service.paused` hoặc `service.deprecated`, M2 consume và tự động đánh dấu package bị ảnh hưởng cần rà soát — nhưng target status khác nhau theo loại sự kiện: `price.changed` → package đang active có entitlement liên quan chuyển ngay về `draft` (gỡ khỏi marketplace, dừng bán, ACE Admin được thông báo cần rà soát lại giá/margin); `service.paused`/`service.deprecated` → package liên quan chuyển về `paused`. Cả hai đều là **hard stop tức thời** — khác với việc PM tự tay sửa một package đang active (M2-US-04 AC3), vốn chỉ fork một version draft chạy song song trong khi version đang live vẫn tiếp tục bán bình thường cho tới khi version mới được duyệt. Cascade không dùng cơ chế fork đó vì giá/chi phí nền tảng đã thay đổi — tiếp tục bán theo giá cũ là đúng vấn đề mà D5 phải ngăn chặn.

**Lý do:** Nếu không có cascade tự động, nhà cung ứng tạm dừng dịch vụ hoặc đổi giá lúc 2 giờ sáng sẽ khiến package chứa entitlement đó tiếp tục bán âm thầm theo dữ liệu lỗi thời. Kafka consumer làm hệ thống tự sửa — thay đổi upstream lan truyền trong < 500ms.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Package tự pending_review/paused khi giá hoặc service thành phần có sự cố — không cần giám sát thủ công | M2 consumer phải idempotent; sự kiện trùng lặp không được double-flag/double-pause |
| PM được thông báo kèm ngữ cảnh cụ thể | Consumer lag trong thời gian sự cố tạo cửa sổ package lỗi thời còn active |

**Luồng khôi phục sau `price.changed` (pending_review → active trở lại):** package bị cascade về
`pending_review` không tự phục hồi — ACE Admin phải chủ động rà soát, và kết quả rẽ theo hai nhánh:

```
Package → draft (do price.changed — hard stop, ACE Admin được
thông báo cần rà soát giá/margin)
     │
     ▼
PM cập nhật margin/selling_price theo giá M1 mới (M1-US-05 / M2-US-05)
Finance Manager cấu hình lại revenue_split cho version mới nếu cần (D7 —
package_items của version mới là row mới, không kế thừa revenue_splits cũ)
     │
     ▼
ACE Admin review (cổng phê duyệt D4 — kiểm tra price sanity, revenue split
sum=100%, platform_fee 5–40%)
     │
     ├── approve ──▶ package_versions v+1 tạo (bất biến — D3): prices/
     │               package_items/revenue_splits mới. current_version_id
     │               → v+1, status = active. event: package.published →
     │               Kafka (M6 catalog nhận version mới, package bán lại
     │               được)
     │
     └── từ chối / cần sửa thêm ──▶ status = draft (chỉ PM thấy — M2-US-04
                     AC2); PM sửa lại rồi submit lại → pending_review, lặp
                     lại vòng review
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

### D10 — Không partition bảng nào ở Phase 1; dùng index thường

**Quyết định:** Không bảng nào trong M1/M2 (`services`, `inventory`, `price_history`, `audit_log` ở cả hai schema) dùng table partitioning ở Phase 1. Thay vào đó: B-tree index trên `airport_code` cho `services`/`inventory`; `EXCLUDE USING GIST` sẵn có (đã định nghĩa trên `service_id, price_type, tstzrange`) phục vụ luôn mục đích lọc cho `price_history`; B-tree index trên `created_at` cho `audit_log`, dùng cho cả truy vấn theo khoảng thời gian lẫn cleanup job (`DELETE ... WHERE created_at < ...` theo batch, thay vì `DROP PARTITION`).

**Lý do:** Partition (LIST theo `airport_code` hoặc RANGE theo `created_at`) đòi hỏi partition key nằm trong mọi PK/UNIQUE constraint của chính bảng đó. Với `price_history`, điều này xung đột trực tiếp với `EXCLUDE USING GIST` hiện có (định nghĩa trên `service_id`, không phải `created_at`). Với `services`/`inventory`/`audit_log`, nên partition PK của riêng bảng đó không kéo theo cascade sang bảng khác; lý do duy nhất còn lại để chưa partition là quy mô Phase 1 chưa đủ lớn để lợi ích partition pruning vượt qua chi phí vận hành thêm (tạo partition mới định kỳ, đảm bảo default partition, giám sát riêng).

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Không cần vận hành thêm: không job tạo partition theo tháng, không default partition cần giám sát | Không có partition pruning ở bất kỳ đâu — mọi query lớn phụ thuộc hoàn toàn vào chất lượng index |
| `price_history` không cần thiết kế lại `EXCLUDE` constraint | `audit_log` cleanup dùng DELETE theo batch thay vì DROP partition — chậm hơn, để lại dead tuple, cần job định kỳ + theo dõi table bloat/VACUUM |

---

### D11 — Transactional Outbox Pattern qua Debezium CDC

**Quyết định:** Mỗi service (M1, M2) có bảng `outbox` trong schema của nó. Khi một use case cần phát sinh Kafka event, thay vì gọi Kafka trực tiếp, service INSERT một row vào `outbox` trong cùng DB transaction với entity change — phần này giữ nguyên so với outbox pattern gốc. Cơ chế relay không phải goroutine tự viết polling mà là **Debezium PostgreSQL Connector** chạy trên **Kafka Connect**, đọc trực tiếp write-ahead log (WAL) của Aurora PostgreSQL qua logical replication slot (`wal_level=logical`) — event-driven theo LSN, không polling. **Outbox Event Router** (Debezium SMT) tự động "bóc" mỗi row `outbox` thành một Kafka message đúng topic/key mà không cần code publish/mark-published thủ công; offset (LSN) do Kafka Connect tự lưu và quản lý resume-after-crash, không cần cột `status` ở tầng DB. Consumer phía nhận vẫn phải idempotent — dùng bảng `processed_events` với `ON CONFLICT DO NOTHING` để bỏ qua duplicate.

**Lý do:** Vẫn không có distributed transaction giữa Aurora PostgreSQL và Amazon MSK, nên nguyên tắc outbox (ghi event cùng transaction với entity change) không đổi. Nhưng relay tự viết bằng polling có ba vấn đề cụ thể: (1) latency cố định ~500ms bất kể outbox đang rỗng hay dồn ứ nhiều thay đổi; (2) phải tự vận hành logic retry/backoff/đánh dấu published, tăng bề mặt code cần maintain; (3) `SELECT ... FOR UPDATE SKIP LOCKED` chạy mỗi 500ms tạo tải liên tục lên Aurora dù không có gì thay đổi. Debezium đọc WAL theo cơ chế push khi WAL advance, loại bỏ cả ba vấn đề trên.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Latency giảm từ ~500ms (chu kỳ poll cố định) xuống mức sub-second, event-driven theo WAL | Thêm hạ tầng mới: Kafka Connect cluster chạy Debezium connector — thành phần vận hành ngoài Aurora + MSK |
| Không cần tự viết/maintain logic polling, retry, đánh dấu published — Kafka Connect tự quản lý offset (LSN) | Logical replication slot giữ WAL không được recycle tới khi Debezium consume xong — connector down kéo dài có thể làm WAL phình to, đầy disk Aurora |
| Schema `outbox` đơn giản hơn — không cần `status`/`retry_count`/`published_at`, không còn `SELECT WHERE status='pending'` liên tục lên DB | Cần alerting/monitoring riêng cho replication slot lag và Kafka Connect connector health — lớp giám sát mới ngoài các dashboard hiện có |
| Không có race condition nhiều instance polling cùng lúc — Debezium connector đọc WAL tuần tự theo LSN, không cần `SKIP LOCKED` | Thay đổi schema trên bảng `outbox` (hoặc publication liên quan) cần cấu hình lại Debezium connector — thêm một điểm phối hợp khi migrate |

---

**Hệ quả bổ sung:**

- **M1 ship trước M2:** M2 chỉ cần API và Kafka events của M1 ổn định. Contract cần freeze trước khi M1 ship: `GET /v1/services/{id}/snapshots` response shape (PM pull khi xây dựng gói — không cần event) và payload Kafka của các sự kiện cascade (`service.paused`, `price.changed`).
- **`opid_ref` nullable trên mọi bảng identity:** Phase 2 identity merge (ACE_UID → OPID) chỉ cần data backfill, không cần ALTER TABLE.
- **Denormalization có chủ đích:** `service_type` trong `package_items` và `supplier_id` trong `revenue_splits` được lưu lại để settlement và analytics query không cần API call về M1.
- **Phase 2 targeting readiness:** `targeting_rules.conditions` đã hỗ trợ field `flight_class`, `loyalty_tier`, `opid_ref` trong JSONB từ Phase 1 — không cần migration khi OPID integration đi vào hoạt động.
