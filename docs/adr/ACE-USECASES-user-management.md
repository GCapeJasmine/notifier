# ACE Platform — Use Case & Flow Specification: User Management Service

> Nguồn: `T2_Tài Liệu Kỹ Thuật.docx` §4.3.1 (trách nhiệm), §5.1.8 (data model), §7.2 (auth flow),
> §7.3 (RBAC matrix). Kiến trúc, DDL và quyết định thiết kế đã phân tích ở
> [`ACE-ADR-0004-user-management.md`](./ACE-ADR-0004-user-management.md) — tài liệu này chỉ mô tả
> luồng nghiệp vụ theo góc nhìn actor, không lặp lại DDL.
>
> Khác với M1/M2 (`ACE-USECASES-m1-m2.md`), T2 không mô tả User Management bằng user story +
> acceptance criteria theo kiểu RFP — chỉ có bảng trách nhiệm chức năng và data model. Các use
> case dưới đây được suy ra từ 4 trách nhiệm cốt lõi ở §4.3.1, không trích dẫn AC cụ thể.

---

## 1. Tổng quan

User Management Service là dịch vụ xuyên suốt (cross-cutting), phục vụ **tất cả** actor trong toàn
bộ nền tảng ACE — không riêng M1/M2. Actor gồm hai nhóm:

- **Generic actor**: bất kỳ user nào đã có tài khoản (staff ACE, supplier, B2B customer) — dùng
  chung luồng đăng nhập, đổi mật khẩu, MFA.
- **11 role nghiệp vụ** (RBAC — §7.3), mỗi role gắn với phạm vi module cụ thể.

| Vai trò | Phạm vi module | Quyền chính |
|---------|-----------------|-------------|
| ACE Super Admin | Tất cả | Toàn quyền CRUD, duyệt NCC/quyết toán, quản lý RBAC |
| ACE Operator | M1, M7 | Hỗ trợ onboarding, kiểm tra dữ liệu, quản lý nội dung |
| Product Manager | M2 | Tạo/sửa gói dịch vụ, nhắm đối tượng, khuyến mãi |
| Finance Manager | M6 Settlement | Xem quyết toán, duyệt chi trả, đối soát |
| Fraud Analyst | M6 Settlement | Quy tắc gian lận, rà soát trường hợp bị đánh dấu, whitelist |
| Partner Manager | M7 | Hợp đồng, SLA, allotment, tuân thủ |
| Supplier Admin | M1 (dữ liệu của chính mình) | Đăng tải dịch vụ, cập nhật tồn kho, xem SLA |
| Supplier Operator | M1 (dữ liệu của chính mình) | Xem dịch vụ, cập nhật tồn kho ở mức hạn chế |
| B2B Customer | M6 Marketplace | Duyệt, giỏ hàng, thanh toán, lịch sử đơn, voucher |
| Kiosk Operator | M6 Voucher | Quét QR/NFC, đánh dấu đã đổi voucher |
| C-Level | Dashboard | Dashboard doanh thu dạng chỉ đọc |

Nguyên tắc thực thi (§7.3): **deny-by-default**, scope theo từng endpoint, lan truyền thay đổi
quyền ≤5 giây, không có đường vòng qua API, lịch sử đầy đủ + rollback tức thì.

---

## 2. UC nhóm 1 — Identity Management

| Use case | Actor | Mô tả |
|----------|-------|-------|
| UC1.1 | ACE Admin / self-service | Tạo tài khoản mới (staff, supplier, B2B customer) |
| UC1.2 | User | Cập nhật hồ sơ cá nhân (không ảnh hưởng đăng nhập — D3) |
| UC1.3 | ACE Admin | Vô hiệu hoá tài khoản (status → suspended/deactivated) |

**Flow — tạo và vô hiệu hoá tài khoản**

```
 ACE Admin / Self-service            User Management Service          Kafka
     │                                      │                              │
     │ 1. Tạo tài khoản: email,             │                              │
     │  account_type (staff/supplier/       │                              │
     │  b2b_customer), thông tin định danh  │                              │
     ├──────────────────────────────────────▶ 2. INSERT users +            │
     │                                      │  user_profiles (transaction  │
     │                                      │  — D3: đảm bảo user không    │
     │                                      │  thiếu profile)              │
     │                                      ├──────────────────────────────▶│
     │                                      │  event: user.registered      │
     │                                      │                              │
     │ 3. Cập nhật hồ sơ (avatar, locale,   │                              │
     │  department...) — KHÔNG khoá row     │                              │
     │  đăng nhập                           │                              │
     ├──────────────────────────────────────▶ UPDATE user_profiles          │
     │                                      ├──────────────────────────────▶│
     │                                      │  event: user.updated         │
     │                                      │                              │
     │ 4. ACE Admin vô hiệu hoá             │                              │
     │  (vi phạm / nghỉ việc / hết hợp tác) │                              │
     ├──────────────────────────────────────▶ status = suspended/          │
     │                                      │  deactivated. Session đang   │
     │                                      │  active bị revoke ngay       │
     │                                      ├──────────────────────────────▶│
     │                                      │  event: user.deleted         │
```

---

## 3. UC nhóm 2 — Authentication & Authorization

| Use case | Actor | Mô tả |
|----------|-------|-------|
| UC2.1 | User | Đăng nhập trực tiếp (mật khẩu tự quản lý) |
| UC2.2 | User | Đăng nhập delegated qua Keycloak (OAuth 2.0/OIDC) |
| UC2.3 | User | Bật/xác thực 2FA (MFA) |
| UC2.4 | ACE Admin / User | Thu hồi session (logout central / force re-login) |

**Flow — hai luồng đăng nhập song song (D2)**

```
 User                        User Management Service          Keycloak
  │                                  │                              │
  │ 1a. Đăng nhập trực tiếp          │                              │
  │  (email + password)              │                              │
  ├──────────────────────────────────▶ external_idp_sub IS NULL     │
  │                                  │ → verify password nội bộ     │
  │                                  │                              │
  │ 1b. HOẶC đăng nhập qua Keycloak  │                              │
  │  (SSO / social login...)         │                              │
  ├──────────────────────────────────┼─────────────────────────────▶│
  │                                  │◀──────── sub claim ───────────┤
  │                                  │ external_idp_sub = sub        │
  │                                  │                              │
  │                          2. (nếu mfa_enabled) yêu cầu OTP/TOTP  │
  │◀──────────────────────────────────┤                              │
  │ 3. Xác thực MFA                  │                              │
  ├──────────────────────────────────▶                              │
  │                                  │                              │
  │                          4. INSERT sessions (access/refresh     │
  │                          token hash, issued_via=direct|keycloak)│
  │                          INSERT login_history (event=login)     │
  │                                  │                              │
  │◀──────── JWT (RS256, TTL 1h) ─────┤                              │
  │  + refresh token                 │                              │
  │                                  │                              │
  │ 5. Mọi API call kèm JWT           │                              │
  ├──────────────────────────────────▶ Istio mTLS + JWT verify tại   │
  │                                  │  mọi service — không đường   │
  │                                  │  vòng nào bỏ qua bước này    │
  │                                  │                              │
  │ 6. Logout (chủ động) HOẶC ACE     │                              │
  │  Admin force revoke              │                              │
  ├──────────────────────────────────▶ UPDATE sessions SET           │
  │                                  │  revoked_at = now()          │
  │                                  │  INSERT login_history         │
  │                                  │  (event=logout)               │
  │                                  │  event: user.logged_out       │
```

Ghi chú: access token hết hạn sau 1 giờ (thu hẹp cửa sổ khai thác nếu bị đánh cắp); refresh token
có thể thu hồi để vô hiệu hoá truy cập tức thì (§7.2).

---

## 4. UC nhóm 3 — RBAC (gán vai trò & quyền)

| Use case | Actor | Mô tả |
|----------|-------|-------|
| UC3.1 | ACE Super Admin | Định nghĩa role + gán permission (`role_permissions`) |
| UC3.2 | ACE Admin / Partner Manager / Supplier Admin | Gán role cho user trong phạm vi tenant của mình |
| UC3.3 | ACE Admin | Gán quyền tạm thời (time-bound, `expires_at`) |
| UC3.4 | System (mọi service) | Kiểm tra quyền tại request time |

**Flow — gán role theo tenant + kiểm tra quyền**

```
 ACE Admin (hoặc Partner Manager/            User Management Service
 Supplier Admin trong phạm vi tenant)
     │                                              │
     │ 1. Chọn user + role_code (vd                 │
     │  "supplier_admin") + tenant_type=            │
     │  'supplier' + tenant_id (supplier_id ở M1)   │
     │  (+ expires_at nếu là quyền tạm thời)        │
     ├───────────────────────────────────────────────▶ INSERT
     │                                              │  user_role_assignments
     │                                              │  event: role.assigned
     │                                              │
     │                                              ▼
     │                              ┌───────────────────────────────┐
     │                              │ Request bất kỳ từ user tới    │
     │                              │ M1/M2/M6/M7:                  │
     │                              │ JOIN user_role_assignments →  │
     │                              │ role_permissions → permissions│
     │                              │ kiểm tra permission_code       │
     │                              │ khớp resource:action đang gọi │
     │                              │ (cache Valkey để đạt ≤5s SLA) │
     │                              └───────────────────────────────┘
     │                                              │
     │ 2. Thu hồi role (vi phạm/hết hạn tự động     │
     │  qua expires_at)                             │
     ├───────────────────────────────────────────────▶ DELETE/expire
     │                                              │  user_role_assignments
     │                                              │  event: permission.changed
     │                                              │  (lan truyền ≤5s — §7.3)
```

---

## 5. UC nhóm 4 — User Sync (đồng bộ dữ liệu cho service khác)

| Use case | Actor | Mô tả |
|----------|-------|-------|
| UC4.1 | M1/M2/M6/M7 (System) | Truy vấn thông tin user theo `user_id` qua API |
| UC4.2 | M1/M2/M6/M7 (System) | Consume Kafka event để đồng bộ local cache/read model |

**Flow — downstream module tiêu thụ user data**

```
 User Management Service                          M1 / M2 / M6 / M7
        │                                                 │
        │◀──────────── GET /v1/users/{user_id} ────────────┤
        │  (đồng bộ đồng bộ, dùng khi cần chi tiết ngay    │
        │  lúc xử lý request — vd hiển thị tên actor trên  │
        │  audit log)                                      │
        │                                                 │
        │  event: user.registered / user.updated /         │
        │  user.deleted / role.assigned /                  │
        │  permission.changed (Kafka, qua Outbox — D7)     │
        ├─────────────────────────────────────────────────▶│
        │                                                 │  Consumer idempotent
        │                                                 │  (processed_events),
        │                                                 │  cập nhật local
        │                                                 │  read-model nếu service
        │                                                 │  cần hiển thị tên/role
        │                                                 │  user mà không gọi API
        │                                                 │  đồng bộ mỗi lần
```

Cùng nguyên tắc CQRS/event-carried state transfer đã áp dụng cho `package.published` ở
`ACE-ADR-0002-m1-m2.md`: service nào cần hiển thị thông tin user tần suất cao (vd danh sách audit
log ở M1/M2, danh sách B2B customer ở M6) nên giữ read-model cục bộ đồng bộ qua event, thay vì gọi
API đồng bộ trên mỗi request.

---

## 6. Bảng truy vết (Traceability matrix)

| UC nhóm | T2 § | Sơ đồ luồng trong tài liệu này |
|---------|------|-----------------------------------|
| Identity Management | §4.3.1 điểm 1 | §2 |
| AuthN/AuthZ | §4.3.1 điểm 2, §7.2 | §3 |
| RBAC | §4.3.1 điểm 3, §7.3 | §4 |
| User Sync | §4.3.1 điểm 4 | §5 |
| Data model (8 entity) | §5.1.8 | Tham chiếu — xem `ACE-ADR-0004-user-management.md` |
