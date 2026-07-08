# ADR-0004: Kiến trúc và Thiết kế Database — User Management Service

---

## Bối cảnh

`T2_Tài Liệu Kỹ Thuật.docx` (§4.3.1, §5.1.8, §7.3) đề xuất **User Management Service** như một
trong hai dịch vụ xuyên suốt (cross-cutting), bên cạnh Integration Service, phục vụ toàn bộ nền
tảng ACE (M1, M2, M6, M7). Đây **không phải** một trong bốn module chính của RFP gốc — đây là
service do đội triển khai đề xuất thêm, vì cả bốn module đều cần một nguồn xác thực/phân quyền
tập trung duy nhất thay vì mỗi module tự quản lý user riêng.

> Nguồn: `T2_Tài Liệu Kỹ Thuật.docx`. Tài liệu này chỉ mô tả entity ở mức narrative (PK/FK, mô tả
> chức năng) — không có DDL. DDL dưới đây do ADR này thiết kế, theo đúng convention đã dùng ở
> [`ACE-ADR-0002-m1-m2.md`](ACE-ADR-0001-m1-m2.md) (schema riêng theo service, UUID PK, audit
> columns, cross-service reference bằng UUID không FK).

Bốn trách nhiệm cốt lõi (§4.3.1):

1. **Identity Management** — tạo, cập nhật, vô hiệu hoá tài khoản; lưu định danh cơ bản (họ tên,
   SĐT, email, loại tài khoản).
2. **AuthN/AuthZ** — tích hợp Keycloak cho OAuth 2.0, JWT RS256, 2FA; cấp/thu hồi token; bảo vệ
   toàn bộ API endpoint trong hệ thống ACE.
3. **RBAC** — định nghĩa và gán role cho từng loại tài khoản (consumer, staff, supplier_admin,
   system_admin...); mỗi role có tập permission riêng trên từng service/endpoint.
4. **User Sync** — cung cấp API cho service khác truy vấn user theo `user_id`; publish sự kiện
   Kafka cho downstream (M1, M2, M6, M7) consume.

Ba thách thức chi phối thiết kế:

- **Xác thực kép** — vừa phải hỗ trợ xác thực trực tiếp, vừa phải cho phép delegate sang Keycloak,
  mà không khoá cứng vào một mô hình IdP duy nhất (§4.3.1 điểm 2; §5.1.8 cột `external_idp_sub`).
- **RBAC đa tenant, giới hạn thời gian** — 11 role trải trên nhiều module (§7.3), một số role chỉ
  áp dụng trong phạm vi một loại tenant cụ thể (supplier, b2b_customer), một số cần time-bound
  grant.
- **Sẵn sàng cho Phase 2 OPID** — service này có thể tái sử dụng làm nguồn PII cho M3 (Identity/
  OPID) ở Phase 2 (ghi chú trong T2), nên schema cần chừa chỗ (`opid_ref` nullable) mà không cần
  migrate lại như pattern đã dùng ở M1/M2.

---

## Phân tích & Quyết định

### D1 — Schema riêng, tenant tham chiếu bằng UUID không FK

**Quyết định:** `user_management` là schema/database riêng, không chia sẻ với M1/M2/M6/M7.
`user_role_assignments.tenant_id` là UUID logic trỏ tới `m1_supply.suppliers.supplier_id` hoặc
`m6_marketplace.b2b_customers.customer_id` tuỳ `tenant_type` — không FK, giống nguyên tắc D1 của
`ACE-ADR-0002-m1-m2.md`.

**Lý do:** User Management là dịch vụ xuyên suốt, được mọi module gọi tới; nếu dùng FK xuyên
schema, việc migrate schema ở M1/M6/M7 sẽ vô tình phá vỡ ràng buộc ở đây — ngược với mục tiêu
"xây trước, không chặn tiến độ module khác" mà T2 đặt ra cho service này (§12.2).

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| User Management deploy độc lập, không phụ thuộc migration của M1/M2/M6/M7 | Validate `tenant_id` tồn tại phải thực hiện ở application layer, không có DB constraint |
| Đúng nguyên tắc đã thiết lập toàn nền tảng (nhất quán với M1/M2) | Query "user thuộc supplier nào" phải join ở tầng ứng dụng, không JOIN SQL trực tiếp |

---

### D2 — Delegated authentication qua `external_idp_sub` nullable

**Quyết định:** `users.external_idp_sub` nullable. `NULL` nghĩa là user xác thực trực tiếp với
User Management Service (mật khẩu tự quản lý); có giá trị nghĩa là xác thực đã delegate sang
Keycloak (OAuth 2.0/OIDC), giá trị là `sub` claim trả về từ Keycloak. Business logic về
lifecycle/role/permission/profile vẫn do service này sở hữu trong cả hai trường hợp — chỉ riêng
bước xác thực mật khẩu là được delegate.

**Lý do:** T2 không chốt cứng một mô hình IdP duy nhất cho toàn bộ Phase 1; một cột nullable duy
nhất cho phép cả hai luồng cùng tồn tại mà không cần hai bảng User riêng biệt, đồng thời không
khoá kiến trúc vào Keycloak nếu sau này đổi IdP.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Một schema User duy nhất phục vụ cả xác thực trực tiếp và delegated | Application layer phải rẽ nhánh logic theo `external_idp_sub IS NULL` ở mọi điểm xác thực |
| Đổi/thêm IdP sau này không cần đổi schema | `UNIQUE INDEX` trên `external_idp_sub` phải là partial index (bỏ qua NULL) — dễ bị quên khi thêm cột tương tự sau này |

---

### D3 — Tách `User` khỏi `User Profile`

**Quyết định:** `users` chỉ giữ các cột trên "đường nóng" (hot path) của đăng nhập/phân quyền:
`email`, `external_idp_sub`, `account_type`, `mfa_enabled`, `status`. Toàn bộ dữ liệu hồ sơ
(`full_name`, `avatar_url`, `locale`, `timezone`, `department`, `title`) nằm ở `user_profiles`,
quan hệ 1–1 qua `user_id UNIQUE`.

**Lý do:** T2 nêu rõ mục đích tách bảng là "giảm kích thước bảng User và cho phép update profile
không khoá row đăng nhập" (§5.1.8) — mỗi lần user tự đổi avatar hay locale không nên tranh chấp
lock với luồng xác thực đang chạy đồng thời.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Update profile (avatar, locale, department...) không khoá row login | Đọc đầy đủ thông tin user cần JOIN 2 bảng thay vì 1 |
| Bảng `users` nhỏ, hot path đăng nhập nhanh hơn khi scale | Phải đảm bảo tạo `user_profiles` đồng thời khi tạo `users` (transaction), tránh user không có profile |

---

### D4 — RBAC 4 bảng: Role, Permission, Role_Permission, User_Role_Assignment

**Quyết định:** RBAC tách 4 bảng độc lập thay vì gán permission thẳng vào user hoặc nhúng cứng
trong code: `roles` (catalog vai trò), `permissions` (catalog quyền hạn dạng `resource:action`),
`role_permissions` (N-N), và `user_role_assignments` (gán role cho user theo `tenant_type` +
`tenant_id`, có `expires_at` cho time-bound grant).

**Lý do:** T2 liệt kê 11 role trải trên nhiều module (§7.3) — một role như `supplier_admin` chỉ có
ý nghĩa trong phạm vi 1 supplier cụ thể, trong khi `ace_super_admin` là toàn cục. Nếu gán quyền
thẳng vào user, thêm một permission mới cho cả nhóm role sẽ phải update hàng loạt user thay vì 1
row trong `role_permissions`.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Thêm/sửa permission cho một role chỉ cần sửa `role_permissions`, áp dụng ngay cho mọi user có role đó | Kiểm tra quyền tại runtime cần JOIN 3-4 bảng — cần cache (Valkey) để đạt mục tiêu lan truyền quyền ≤5s (§7.3) |
| `tenant_type` + `tenant_id` + `expires_at` hỗ trợ cả role toàn cục, role theo tenant, và quyền tạm thời trong cùng một bảng | `UNIQUE (user_id, role_id, tenant_type, tenant_id)` phải xử lý đúng khi `tenant_id IS NULL` (role toàn cục) — Postgres coi nhiều NULL là phân biệt, cần lưu ý khi enforce uniqueness |

---

### D5 — `Session` (mutable, revocable) tách khỏi `Login History` (append-only)

**Quyết định:** `sessions` lưu token hash và trạng thái sống của phiên đăng nhập hiện tại — có thể
`revoked_at` để logout central hoặc force re-login. `login_history` là bảng append-only, không
UPDATE/DELETE, ghi mọi sự kiện (`login`, `logout`, `login_failed`, `password_change`,
`mfa_setup`) — cùng nguyên tắc với `m1_supply.audit_log`/`m2_package.audit_log` trong ADR-0002.

**Lý do:** Session cần mutable vì phải revoke được ngay lập tức (§7.2: "refresh token có thể thu
hồi để vô hiệu hoá truy cập tức thì"); nhưng lịch sử đăng nhập phục vụ compliance audit thì không
được sửa/xoá — hai yêu cầu đối lập nên tách bảng thay vì dùng chung một bảng với soft-delete.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Revoke session tức thời (`UPDATE sessions SET revoked_at = now()`) không đụng tới audit trail | Ghi 2 bảng cho mỗi lần login (1 session + 1 login_history row) |
| `login_history` không thể bị sửa/xoá — đáp ứng yêu cầu audit compliance | `login_history` tăng trưởng không giới hạn theo thời gian — cần archival/partition khi vượt ngưỡng, cùng vấn đề đã nêu ở D10 của ADR-0002 |

---

### D6 — Chuẩn hoá `account_type` theo `tenant_type` đã có

**Quyết định:** §4.3.1 mô tả cột "loại tài khoản" là `B2B/B2C/Staff`, nhưng `user_role_assignments`
(§5.1.8) đã định nghĩa `tenant_type ENUM (ace, supplier, b2b_customer)` chi tiết hơn — tách riêng
`supplier` khỏi B2B chung chung, vì Supplier Admin/Operator (M1) và B2B Customer (M6 Marketplace)
là hai actor khác hẳn nhau trong bảng RBAC (§7.3). ADR này chuẩn hoá `users.account_type` theo
đúng 3 giá trị của `tenant_type`, cộng thêm `consumer` (Phase 2, hiện chưa dùng — B2C trực tiếp là
tính năng Phase 2 theo M2-US-09) để không phải ALTER TABLE khi B2C kích hoạt.

**Lý do:** Dùng chung một bộ vocabulary (`account_type` ~ `tenant_type`) tránh việc một user có
`account_type = 'B2B'` nhưng `user_role_assignments.tenant_type = 'supplier'` — hai cột mô tả cùng
một khái niệm nhưng lệch nhau, dễ gây bug ở tầng ứng dụng khi kiểm tra quyền.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Một vocabulary duy nhất cho "loại tài khoản" xuyên suốt schema, không lệch giữa `users` và `user_role_assignments` | Diễn giải hẹp hơn nguyên văn T2 ("B2B/B2C/Staff") — cần ghi chú rõ trong tài liệu để tránh hiểu nhầm khi đối chiếu ngược lại T2 |
| `consumer` có sẵn trong enum từ Phase 1, Phase 2 bật B2C không cần ALTER TABLE | Enum có giá trị chưa dùng (`consumer`) trong Phase 1 — cần comment rõ lý do để không bị coi là dead code |

---

### D7 — Chuẩn hoá tên sự kiện Kafka

**Quyết định:** T2 tự mâu thuẫn về tên event ở hai chỗ: §4.3.1 điểm 4 viết `user.created`,
`user.updated`, `user.deactivated`; trong khi bảng tổng quan §4.3 và mô tả entity `User` ở §5.1.8
đều thống nhất dùng `user.registered`, `user.updated`, `user.deleted`, `user.logged_in`,
`user.logged_out`, cộng thêm `role.assigned`, `permission.changed`. ADR này chọn bộ tên ở **§4.3 +
§5.1.8** làm canonical (2/3 lần xuất hiện đồng thuận), vì đây cũng là bộ tên xuất hiện trong bảng
tổng quan service — nơi các service khác (M1, M2, M6, M7) tra cứu để biết event nào cần subscribe.

**Lý do:** Một cái tên event sai lệch giữa tài liệu và implementation sẽ khiến consumer subscribe
nhầm topic — cùng loại lỗi đã gặp với `pending_review`/`pending_approval` ở ADR-0002/ADR-0003.
Chốt danh sách chuẩn ngay trong ADR để tránh lặp lại.

**Đánh đổi:**

| Lợi ích | Chi phí |
|---------|---------|
| Một nguồn sự thật duy nhất cho tên event, khớp với bảng tổng quan service dùng để tích hợp | Khác với câu chữ ở §4.3.1 điểm 4 của T2 gốc — cần đối chiếu ngược khi có thay đổi từ T2 |
| Publish qua Transactional Outbox (cùng pattern D11 của ADR-0002) đảm bảo at-least-once delivery | Cần bảng `outbox` + `processed_events` riêng cho `user_management`, thêm một service nữa phải vận hành cơ chế này |

**Danh sách event chuẩn:** `user.registered`, `user.updated`, `user.deleted`, `user.logged_in`,
`user.logged_out`, `role.assigned`, `permission.changed`.

---

## DDL — Schema `user_management`

```sql
CREATE SCHEMA user_management;

-- ----------------------------------------------------------------
-- USERS
-- external_idp_sub NULL = xác thực trực tiếp; có giá trị = delegate
-- sang Keycloak (D2). account_type chuẩn hoá theo tenant_type (D6).
-- opid_ref nullable — sẵn sàng cho M3 OPID Phase 2, cùng pattern đã
-- dùng ở m1_supply.suppliers/services trong ADR-0002.
-- ----------------------------------------------------------------
CREATE TABLE user_management.users (
    user_id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    email             VARCHAR(255) NOT NULL UNIQUE,
    external_idp_sub  VARCHAR(255),
    account_type      VARCHAR(20)  NOT NULL
                          CHECK (account_type IN ('staff','supplier','b2b_customer','consumer')),
    mfa_enabled       BOOLEAN      NOT NULL DEFAULT false,
    status            VARCHAR(20)  NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active','suspended','deactivated')),
    opid_ref          VARCHAR(100),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ON user_management.users (external_idp_sub) WHERE external_idp_sub IS NOT NULL;
CREATE INDEX ON user_management.users (account_type, status);

-- ----------------------------------------------------------------
-- USER PROFILES (D3 — tách khỏi users, quan hệ 1..1)
-- ----------------------------------------------------------------
CREATE TABLE user_management.user_profiles (
    profile_id   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID         NOT NULL UNIQUE REFERENCES user_management.users,
    full_name    VARCHAR(255),
    avatar_url   TEXT,
    locale       VARCHAR(10)  NOT NULL DEFAULT 'vi',
    timezone     VARCHAR(50)  NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
    department   VARCHAR(100),
    title        VARCHAR(100),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------
-- ROLES
-- scope='global' (vd ace_super_admin) hoặc 'tenant' (vd
-- supplier_admin — chỉ có ý nghĩa gắn với 1 tenant_id cụ thể).
-- ----------------------------------------------------------------
CREATE TABLE user_management.roles (
    role_id     UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    role_code   VARCHAR(50)  NOT NULL UNIQUE,
    name        VARCHAR(100) NOT NULL,
    scope       VARCHAR(20)  NOT NULL DEFAULT 'global'
                    CHECK (scope IN ('global','tenant')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------
-- PERMISSIONS
-- permission_code dạng resource:action, vd supplier:read,
-- settlement:approve — độc lập với role để tái sử dụng (D4).
-- ----------------------------------------------------------------
CREATE TABLE user_management.permissions (
    permission_id    UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    permission_code  VARCHAR(100) NOT NULL UNIQUE,
    description      TEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------
-- ROLE PERMISSIONS (join N-N)
-- ----------------------------------------------------------------
CREATE TABLE user_management.role_permissions (
    id             UUID  PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id        UUID  NOT NULL REFERENCES user_management.roles,
    permission_id  UUID  NOT NULL REFERENCES user_management.permissions,

    UNIQUE (role_id, permission_id)
);

CREATE INDEX ON user_management.role_permissions (permission_id);

-- ----------------------------------------------------------------
-- USER ROLE ASSIGNMENTS
-- tenant_id UUID logic, KHÔNG FK (D1) — tenant sống ở schema khác
-- (m1_supply.suppliers, m6_marketplace.b2b_customers...). NULL khi
-- tenant_type='ace' (role toàn cục). expires_at cho time-bound grant.
-- ----------------------------------------------------------------
CREATE TABLE user_management.user_role_assignments (
    assignment_id  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID         NOT NULL REFERENCES user_management.users,
    role_id        UUID         NOT NULL REFERENCES user_management.roles,
    tenant_type    VARCHAR(20)  NOT NULL
                       CHECK (tenant_type IN ('ace','supplier','b2b_customer')),
    tenant_id      UUID,
    granted_by     UUID         NOT NULL,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),

    UNIQUE (user_id, role_id, tenant_type, tenant_id)
);

CREATE INDEX ON user_management.user_role_assignments (user_id);
CREATE INDEX ON user_management.user_role_assignments (tenant_type, tenant_id);
CREATE INDEX ON user_management.user_role_assignments (expires_at) WHERE expires_at IS NOT NULL;

-- ----------------------------------------------------------------
-- SESSIONS (mutable, revocable — D5)
-- Lưu hash của token, không lưu token gốc. issued_via phân biệt
-- session tạo trực tiếp hay song song khi Keycloak issue token (D2).
-- ----------------------------------------------------------------
CREATE TABLE user_management.sessions (
    session_id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID         NOT NULL REFERENCES user_management.users,
    access_token_hash    VARCHAR(255) NOT NULL,
    refresh_token_hash   VARCHAR(255) NOT NULL,
    issued_via           VARCHAR(20)  NOT NULL DEFAULT 'direct'
                             CHECK (issued_via IN ('direct','keycloak')),
    expires_at           TIMESTAMPTZ  NOT NULL,
    revoked_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON user_management.sessions (user_id, revoked_at);
CREATE INDEX ON user_management.sessions (expires_at) WHERE revoked_at IS NULL;

-- ----------------------------------------------------------------
-- LOGIN HISTORY (append-only — D5, cùng pattern audit_log ADR-0002)
-- ----------------------------------------------------------------
CREATE TABLE user_management.login_history (
    history_id       UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID         NOT NULL REFERENCES user_management.users,
    event_type       VARCHAR(20)  NOT NULL
                         CHECK (event_type IN (
                             'login','logout','login_failed',
                             'password_change','mfa_setup'
                         )),
    failure_reason   TEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX ON user_management.login_history (user_id, created_at);

-- ----------------------------------------------------------------
-- OUTBOX (Transactional Outbox — cùng pattern D11 của ADR-0002)
-- ----------------------------------------------------------------
CREATE TABLE user_management.outbox (
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

CREATE INDEX ON user_management.outbox (status, created_at) WHERE status = 'pending';
```

---

## Hệ quả

| Khía cạnh | Đảm bảo | Lưu ý |
|-----------|---------|-------|
| Xác thực kép | `external_idp_sub` nullable cho phép trực tiếp lẫn Keycloak cùng tồn tại (D2) | Migration IdP trong tương lai chỉ cần backfill cột, không đổi schema |
| RBAC đa tenant, time-bound | 4-bảng RBAC + `expires_at` phủ mọi trường hợp role toàn cục/tenant/tạm thời (D4) | Cần cache permission (Valkey) để đạt SLA lan truyền quyền ≤5s đã nêu ở §7.3 |
| Sẵn sàng OPID Phase 2 | `opid_ref` nullable trên `users`, không cần ALTER TABLE khi M3 kích hoạt | Khung mã hoá AES-256 cấp cột và identity linking framework nằm ngoài phạm vi schema này (xem T2 §12.2, tham chiếu ADR-006) |
| Độ tin cậy message | Outbox pattern (cùng D11 ADR-0002) đảm bảo at-least-once cho 7 event chuẩn hoá ở D7 | Consumer ở M1/M2/M6/M7 phải idempotent — cùng yêu cầu `processed_events` đã áp dụng toàn nền tảng |

**Vai trò chiến lược (T2 §12.2):** User Management Service được xây trong T1–T4, go-live tại MS2
(tháng 4) với đầy đủ RBAC 11 role + OAuth2 + JWT RS256 + audit log — đi vào production **trước**
khi M6/M7 hoàn thiện, để không trở thành điểm nghẽn khi các module phụ thuộc vào xác thực/phân
quyền tập trung. Đây là điều kiện kiến trúc bắt buộc cho M3 OPID, B2C trực tiếp và multi-tenant
white-label ở Phase 2.
