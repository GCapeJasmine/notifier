# CARxRENTAL — Tài liệu Onboarding v1.0

> **Mục đích:** giúp bạn — dev backend / dev frontend / QC / Scrum Master / BA mới join
> CARxRENTAL — nắm **100%** cách hệ thống hoạt động trong **2–3 ngày**.
>
> Phiên bản này là **bản CARxRENTAL-specific** dẫn xuất từ template `ADLC-ONBOARDING-v4.5.md`.
> Template mô tả *khung* ADLC v4.5 chung; file này *điền nội dung thật* của CARxRENTAL
> (personas, boundary, stack, contract, wave sequence đã chốt).
>
> ⚠️ **Bối cảnh khi đọc**: dự án đã ở stage **`DISCOVERED`** (đã hoàn tất D0→D7 + transition
> DISCOVERED; `/sync-to-specs v1.0` xong 2026-07-13 — 101 file artifact đẩy sang SPECS repo;
> `/sync-to-domain v1.0` xong 2026-07-13 — 39 file business context đẩy sang DOMAIN repo).
> 5 role trong tài liệu này (**BE / FE / QC / SM / BA**) chủ yếu vào cuộc ở **pha execution**
> (sau `DISCOVERED`) — nhưng bạn **phải đọc pha DISCOVERY trước** để hiểu "hiến pháp" nào đã
> được đông cứng và bạn không được đụng.
>
> Vị trí: `ONBOARDING-CARXRENTAL-v1.md` (root repo `carxrental-adlc-discovery`).
>
> **Audit + refresh 2026-07-14**: đồng bộ nội dung với source hiện tại (stage DISCOVERED,
> 3 experience, 12 event-storming domain, 60 contract ACTIVE, 58 CR/ADR, 7-wave sequence,
> chuỗi 3 bên DriverPlus → CARxRENTAL → ACN — INV-001..005).

---

## 🧭 BẢN ĐỒ TÀI LIỆU — đọc theo thứ tự này

Tài liệu chia **3 phần**. Người mới đọc **PHẦN A trước** (bối cảnh sản phẩm CARxRENTAL);
sau đó nhảy vào **PHẦN B — cẩm nang role của bạn**; **PHẦN C** để tra cứu cơ chế khi cần đào sâu.

| | Mục | Trả lời câu hỏi |
|---|---|---|
| **PHẦN A — SẢN PHẨM CARxRENTAL** | §0 Bức tranh sản phẩm | "CARxRENTAL là gì, phục vụ ai, kiếm tiền thế nào?" |
| *(đọc trước, ~2h)* | §1 Dàn nhân vật | "Ai duyệt, agent AI nào chạy, hook nào canh?" |
| | §2 DISCOVERY đã đông cứng | "D0–D7 đã chốt cái gì? Cái nào bất biến?" |
| | §3 EXECUTION — Wave sequence | "Sau DISCOVERY, thứ tự làm việc theo Wave nào?" |
| | §4 Kiến trúc CARxRENTAL | "12 boundary + 3 BFF + 3 experience — chia stack gì?" |
| | §5 Contracts | "60 contract chia 4 loại — vì sao?" |
| **PHẦN B — CẨM NANG 5 ROLE** | §6 Dev Backend | "Tôi code BE — làm gì, ở đâu, đọc gì trước?" |
| *(sau A, ~2h/role)* | §7 Dev Frontend | "Tôi code FE (web/mobile) — làm gì?" |
| | §8 QC | "Tôi test — chạy gì, phán quyết thế nào?" |
| | §9 Scrum Master | "Tôi điều phối wave — kênh nào, cadence nào?" |
| | §10 BA | "Tôi viết nghiệp vụ — viết gì, cấm chữ nào?" |
| **PHẦN C — CƠ CHẾ + TRA CỨU** | §11 Hooks + Gate + STATE | "Máy tự áp luật thế nào?" |
| *(khi cần, ~1h)* | §12 Truy vết end-to-end | "1 dòng code truy ngược tới đâu?" |
| | §13 Người duyệt + Cấp CR | "Muốn đổi thì ai duyệt?" |
| | §14 Lộ trình học + tra cứu | "2–3 ngày đọc theo thứ tự nào?" |
| | §15 FAQ CARxRENTAL | "Vì sao chọn Go+NestJS, Elastic, Keycloak…?" |

---

# ═══════════ PHẦN A — CARxRENTAL LÀ GÌ ═══════════

## §0. Bức tranh sản phẩm

### 0.1 CARxRENTAL — 2 sản phẩm độc lập bổ trợ nhau

**CARxRENTAL** cung cấp thị trường cho thuê xe tại Việt Nam **hai sản phẩm tách biệt về giá
trị**, có thể dùng riêng hoặc kết hợp:

| Sản phẩm | Mô hình doanh thu | Khách chính | Chỉ số Bắc Đẩu |
|---|---|---|---|
| **SaaS Fleet Management** | Subscription **MRR/tenant/tháng** (multi-tenant pooled) | Fleet operator (B2B) — điều hành nội bộ kể cả không lên Sàn | MRR, số xe active, retention Fleet |
| **Marketplace (Sàn)** | **Take-rate GMV** (chốt ở PolicyConfig) | Fleet + Consumer thuê qua Sàn | GMV, số booking, giá trị đơn TB |

**Quan hệ 2 sản phẩm:** Fleet có thể chỉ dùng SaaS để vận hành nội bộ (khách trực tiếp / walk-in
/ khách quen, `channel=internal`), không cần đưa xe lên Sàn. Ngược lại, một số nguồn cung Sàn
(P2P, B2D) không nhất thiết là khách SaaS. Fleet SaaS là **sản phẩm bảo hiểm** — có thể ship
standalone kể cả khi Track B Sàn trượt (xem §3 wave sequence, W2 milestone bảo hiểm).

### 0.2 Chuỗi phân phối 3 bên — DriverPlus → CARxRENTAL → ACN

Điểm khác biệt cốt lõi so với các marketplace cho thuê xe thuần online (Mioto, Chungxe): CARxRENTAL
là **nguồn cung xe** trong chuỗi phân phối 3 bên đã chốt ở **ADR-CR-040** (5 Invariant INV-001..005):

```
Consumer → DriverPlus (app Consumer external) → CARxRENTAL (Sàn cấp nguồn xe) → ACN Service Station (điểm phục vụ + giao–nhận vật lý gần khách nhất)
```

**5 Invariant không thương lượng** (đổi cần `/cr-raise CRITICAL` + Authority sign):

| ID | Invariant | Ý nghĩa |
|---|---|---|
| **INV-001** | Consumer touch point cho đơn qua kênh ACN = **DriverPlus** (external app). CARxRENTAL nhận đơn qua API contract; Consumer là external actor với CARxRENTAL. | CARxRENTAL không có app Consumer riêng — DriverPlus là điểm chạm duy nhất. |
| **INV-002** | CARxRENTAL cấp nguồn xe (F2D/P2P/B2D) cho đơn từ DriverPlus; **ACN Service Station gần khách nhất** = điểm phục vụ + giao–nhận + xác minh vật lý. | Tách bạch: Sàn giữ supply + matching; ACN giữ hạ tầng vật lý địa phương. |
| **INV-003** | Phân định xác minh: **ACN làm KYC Consumer**; **CARxRENTAL làm KYB Fleet**. CARxRENTAL nhận trust level Consumer từ ACN qua contract (`acn-consumer-trust-v1` — ADR-CR-046). | Mỗi trách nhiệm xác minh có một chủ duy nhất; tránh trùng KYC hoặc lỗ hổng. |
| **INV-004** | SoT tiền + revenue-share dọc chuỗi chốt theo lớp: phí Sàn CARxRENTAL, hoa hồng ACN, doanh thu Fleet — mỗi lớp một owner tính toán duy nhất. | Tránh nhiều bên tự tính lệch số. |
| **INV-005** | Luồng Fleet tự vận hành nội bộ (`channel=internal`, khách trực tiếp, KHÔNG qua Sàn) **nằm ngoài chuỗi này** — giữ nguyên per ADR-CR-002..008/034/037. | SaaS Fleet Management là sản phẩm độc lập. |

Chi tiết: `_discovery/hypothesis-log.md §2.5-2.6`.

### 0.3 Beachhead F2D — Fleet trước, P2P/B2D sau

Sàn Marketplace hỗ trợ **3 mô hình supply**, ship theo thứ tự:

| Mô hình | Nghĩa | Phase |
|---|---|---|
| **F2D** (Fleet → Driver) | Fleet operator cho khách lẻ (Consumer + Driver) thuê qua Sàn | **Phase 1 MVP — beachhead** |
| **P2P** (Driver → Driver) | Chủ xe cá nhân cho thuê trực tiếp qua Sàn | Phase 2 |
| **B2D** (Brand → Member) | Thương hiệu/doanh nghiệp cho tập member của mình thuê | Phase 2 |

Lý do beachhead F2D: Fleet operator là nguồn cung có kiểm soát (đã KYB), làm liquidity anchor
trước khi mở P2P (biến số nhiều hơn về chất lượng xe + trust).

### 0.4 9 personas + 2 external actor + anti-personas

**9 personas ACTIVE** trong `_discovery/persona-pool.md`:

| Nhóm | ID persona | Vai trò |
|---|---|---|
| **Fleet-side** (2) | `P-FLEETOWNER` | Chủ Fleet (kiêm vận hành) — quản lý xe, availability, allotment, hợp đồng thuê nội bộ, tài chính nội bộ |
| | `P-FLEETACCOUNTANT` | Kế toán Fleet — thu chi nội bộ, đối soát InternalReceipt, P&L per xe (Phase 2) |
| **Renter-side** (3) — external actor via DriverPlus | `P-RENTER-SELFDRIVE` | Renter tự lái |
| | `P-RENTER-CHAUFFEUR` | Renter thuê xe kèm tài xế |
| | `P-DRIVER` | Tài xế cơ hữu của Fleet (Phase 1) — tài xế thời vụ defer Phase 2 |
| **Platform-side** (4) | `P-PLATFORMOPS` | Vận hành Sàn — KYB approve fleet, dispute, listing QC, PolicyConfig SoT |
| | `P-CS` | Customer Support |
| | `P-TENANTOPS` | Quản lý Tenant / Fleet Onboarding — provisioning + subscription state |
| | `P-PLATFORMFINANCE` | Tài chính Sàn — settlement, take-rate, GMV reconcile |

**2 External Actor** (không phải persona, nhưng phải hiểu vì chốt luồng dữ liệu):

| Actor | Vai trò |
|---|---|
| **DriverPlus** | App Consumer external — điểm chạm Consumer + inbound RentalRequest cho Sàn (INV-001) |
| **ACN Service Station Operator** | Mạng lưới trạm dịch vụ vật lý — KYC Consumer + giao/nhận xe + service scope area (INV-002/003) |

**7 Anti-persona** (BA cần biết để giữ scope — CARxRENTAL cố tình KHÔNG phục vụ):
1. Người thuê để cầm cố / trục lợi / chiếm đoạt xe (tail-risk hình sự)
2. Khách thuê ngoài Việt Nam (out-of-scope beachhead)
3. Chủ xe cá nhân P2P host (Phase 2+, không thiết kế UI/workflow ở MVP)
4. Brand/Member B2D (Phase N — vision level, chưa hypothesis định lượng)
5. CARxRENTAL tự sở hữu fleet (anti-hypothesis: platform là marketplace + SaaS + phân phối, không phải first-party fleet owner)
6. Đặt xe một chiều không trả về station gốc (operational feasibility với ACN chưa rõ — out-of-scope MVP)
7. Consumer dùng app CARxRENTAL riêng để thuê qua kênh ACN (INV-001 — Consumer touch point là DriverPlus)

Chi tiết: `_discovery/persona-pool.md §Anti-Personas`.

### 0.5 Timeline DISCOVERY

DISCOVERY chạy **7 ngày thực tế** (D0 = 2026-07-07 → DISCOVERED signoff = 2026-07-13). Chấp
nhận được vì Architecture Authority (Nguyen Ha Anh) đồng thời giữ Business + Delivery + Security
role → ra quyết định nhanh, không chờ cross-team review. Đánh đổi: rủi ro tập trung 1 người
→ phải bù bằng QA độc lập (§13). Đã sync xong sang SPECS (v1.0, 101 file) + DOMAIN (v1.0,
39 file) ngày 2026-07-13.

### 0.6 Framework — ADLC v4.5

CARxRENTAL vận hành trên **ADLC v4.5** (*Agentic Development Life Cycle*): dây chuyền sản xuất
phần mềm điều khiển bằng AI agent. Đầy đủ có **7 phase repo** (DISCOVERY + DOMAIN + ARCHITECT
+ UIUX + SPECS + TESTING + ORCHESTRATOR) + **2 code repo** (`-BOUNDARIES`, `-EXPERIENCES`).
Hiện CARxRENTAL có:
- **DISCOVERY** (repo này) — DISCOVERED, đã đông cứng
- **SPECS** — nhận artifact từ DISCOVERY qua `/sync-to-specs v1.0` (2026-07-13)
- **DOMAIN** — nhận business context từ DISCOVERY qua `/sync-to-domain v1.0` (2026-07-13)
- **ARCHITECT / UIUX / TESTING / ORCHESTRATOR / -BOUNDARIES / -EXPERIENCES** — sẽ bootstrap tiếp
  từ SPECS/DOMAIN ở bước execution.

---

## §1. Dàn nhân vật CARxRENTAL

Dây chuyền có 3 *loại* tác nhân: **con người** (quyết & ký), **AI agent** (làm phần nặng),
**máy móc** (hook/gate canh luật tự động).

### 1.1 Con người — 5 người duyệt gác cổng

| Vai trò | Người | Gác/ký ở CARxRENTAL |
|---|---|---|
| **Kiến trúc** | Nguyen Ha Anh `<nguhanh@cardoctor.vn>` | Stack polyglot Go+NestJS (**ADR-D3-001** + Echo CR-023 + GORM CR-018), BFF NestJS+Apollo (CR-024), CONTRACT_READY_GATE, HLD |
| **Delivery** | Nguyen Ha Anh (kiêm) | WAVE-SEQUENCE 7-wave / 7 tuần / team 10 (ADR-D7-001), Dual-Track A(Fleet)/B(Sàn), `/wave-start`, `/sign-qc` |
| **Business** | Nguyen Ha Anh (kiêm) | Hypothesis-log + 5 Invariant INV-001..005 (ADR-CR-040), 9 personas + 2 external actor, 74 capabilities, `/wave-domain-ready` |
| **QA** | `qa@cardoctor.vn` **(BẮT BUỘC ≠ Business)** | `/wave-signoff` → APPROVED / CONDITIONAL / REJECTED |
| **Bảo mật** | Nguyen Ha Anh (kiêm) | Keycloak self-host EKS (CR-020), Elastic Observability (CR-027 + CR-030 trim Metricbeat), review trước DEV |

> ⚠️ **Rủi ro concentration:** Nguyen Ha Anh hiện giữ **4/5 vai** duyệt (Kiến trúc + Delivery
> + Business + Bảo mật). Chấp nhận được vì tốc độ, nhưng **QA BẮT BUỘC phải là người khác** —
> nguyên tắc "vừa đá bóng vừa thổi còi" không được vi phạm ở cổng ship. QA lead: `qa@cardoctor.vn`.

### 1.2 AI agent — theo phase

**DISCOVERY (đã dùng — D0→D7):**
- `meta` — nhạc trưởng, spawn agent khác
- `agent-capability-mapper` (D1)
- `agent-event-stormer` (D2 — 1 spawn/domain, tổng 12 spawn)
- `agent-charter-author` (D3 skeleton, D5 CHARTER đầy đủ, D7 WAVE-SEQUENCE)
- `agent-standards-enricher` (D3.5 — điền `_shared/*`)
- `agent-contract-steward` (D4 — 1 spawn/kind)
- `agent-aggregator` (D6 — render `_aggregated/*`)

Agent spec ở `.claude/agents/*.md`.

**EXECUTION (sau `DISCOVERED` — sẽ bootstrap):**
- DOMAIN: `po-author`, `ba-author`, `domain-translator`
- ARCHITECT: `sa-author`, `adr-author`, `architect-translator`
- UIUX: `ds-author`, `uiux-translator`
- SPECS: `contract-steward` (mode SPECS), `meta` (wave orchestration)
- TESTING: `qa-author`, `qa-executor`, `qa-translator`
- BOUNDARIES/EXPERIENCES: `dev-backend`/`dev`, `fix-*`, `review-*`, `test-unit`, `test-contract`, `test-integration`/`test-component`

**Bộ 3 vai lặp lại ở mọi trạm:** **meta điều phối → author viết → translator dịch sang SPECS.**
Nắm 3 vai này là đọc được mọi repo.

### 1.3 Máy móc — hook đang active

Hook enforcement **tự động chạy**, agent AI không thể "đi tắt":

| Hook (file) | Chặn cái gì |
|---|---|
| `sessionstart.sh` (SessionStart) | In STATE + NON-NEGOTIABLES đầu session |
| `userpromptsubmit.sh` (UserPromptSubmit) | Inject `[STATE: W0/DISCOVERED/agent=meta/…]` mỗi turn |
| `pretooluse-meta-edit-block.sh` | Chặn meta Edit/Write ngoài allowlist (ONBOARDING-*.md exempt vì ngoài framework) |
| `pretooluse-boundary-block.sh` | Chặn edit ngoài `owned_paths` của agent hiện tại |
| `pretooluse-aggregated-readonly.sh` | Chỉ `aggregator` được ghi `_aggregated/**` |
| `pretooluse-question-budget.sh` | Chặn tool call khi agent vượt question budget (FM-QUESTION-BUDGET) |
| `pretooluse-question-budget-floor.sh` | Enforce budget tối thiểu — reset về floor nếu bị set âm |
| `precompact.sh` (PreCompact) | Pin STATE + 3 decision mới nhất vào compaction summary |

**Gate script** (chạy qua `state.py transition`, không phải PreToolUse hook):
- `discovery-gate.py <D-wave>` — check exit gate D-wave trước transition; fail → block. Override:
  `state.py transition <Y> --force --reason "<lý do>"` (audit row).

**Chưa enable ở CARxRENTAL (audit gap):**
- `pretooluse-readonly-inputs` — chặn ghi `_inputs/**` (sẽ enable ở DOMAIN/ARCHITECT sau bootstrap).
- `contract-hash-check` — re-hash contract mỗi lần đụng source, block FM-CONTRACT-DRIFT (sẽ enable
  ở SPECS sau bootstrap).

Chi tiết: `.claude/settings.json` + `scripts/hooks/`.

---

## §2. DISCOVERY đã đông cứng (D0 → D7)

> 🔒 Đây là "hiến pháp" đã đông cứng. **5 role execution (BE/FE/QC/SM/BA) không được sửa
> những cái này ngoài quy trình CR có người ký (§13).** Chỉ cần đọc để hiểu.

Ngắn gọn, mỗi D-wave đã chốt cái gì:

| Sub-wave | CARxRENTAL đã chốt cái gì | File chính |
|---|---|---|
| 🟦 **D0** | Vision + problem statement + Hypotheses + Anti-hypotheses + **5 Invariant INV-001..005** (chuỗi DriverPlus/CARxRENTAL/ACN — ADR-CR-040) | `_discovery/hypothesis-log.md` |
| 🟦 **D1** | 9 personas + 2 external actor (DriverPlus, ACN) + **74 capabilities** (mỗi CAP gắn Phase 1 / Phase 2 priority + candidate domain) | `_discovery/persona-pool.md`, `_discovery/capability-map.md` |
| 🟦 **D2** | Event storming **12 domain** (1 ES / boundary): booking, driver-management, fleet-core, fleet-finance, handover, identity-verification, marketplace, notification, payment, platform-ops, reputation, tenant-management | `_discovery/event-storming/ES-*.md` |
| 🟨 **D3** | **12 backend boundary + 3 BFF + 3 experience**; 30+ ADR/CR chốt stack (§4 chi tiết) | `BOUNDARY-MAP.md`, `SYSTEM-TOPOLOGY.md`, `Execution/tracking/decisions.md` |
| 🟨 **D3.5** | 5 file coding-standards enriched (`backend-golang.md`, `backend-nodejs.md`, `bff.md`, `frontend-web.md`, `frontend-mobile.md`) + 3 policy (observability, release, security) | `_shared/*`, `_shared/ENRICHMENT-LOG.md` |
| 🟧 **D4** | **60 contract ACTIVE** (api 15 + event 26 + ui 3 + data 6) + **130 consumer signatures** (ADR-CR-039); ADR-CR-036 chốt AllotmentPool + POST /availability/hold atomic idempotent | `contracts/{api,event,ui,data}/*`, `CONTRACT-MAP.md` |
| 🟩 **D5** | **18 CHARTER ACTIVE** (12 backend + 3 BFF + 2 web + 1 mobile); ADR-CR-048 fix 39 deep-review findings | `boundaries/*/CHARTER.md`, `web-experiences/*/CHARTER.md`, `mobile-experiences/*/CHARTER.md` |
| 🟧 **D6** | 4 render aggregated: PRD + ROADMAP + SYSTEM-ARCHITECTURE + TECHSTACK | `_aggregated/*.md` |
| 🟥 **D7** | `plan/WAVE-SEQUENCE.md` — Dual-Track Parallel + MoSCoW Timeboxed + Walking Skeleton, **7 wave / 7 tuần / team 10** (ADR-D7-001, ADR-CR-049 MVP coverage) | `plan/WAVE-SEQUENCE.md` |
| ⬛ **DISCOVERED** | Đã transition; `/sync-to-specs v1.0` xong (101 file → SPECS) + `/sync-to-domain v1.0` xong (39 file → DOMAIN), cả hai ngày 2026-07-13 | `Execution/STATE.json` |

> ✅ **Checklist đọc DISCOVERY (~1h):** open `_aggregated/PRD.md`, đọc §problem + §personas + §capabilities.
> Sau đó `plan/WAVE-SEQUENCE.md` để thấy thứ tự làm. Đó là 2 file "hiến pháp" bạn cần thuộc.

---

## §3. EXECUTION — Wave sequence CARxRENTAL

### 3.1 Wave concept (recap)

Một **Wave** = một lát sản phẩm đi trọn vòng *nghiệp vụ → ship*. Mỗi wave chốt bằng **2 trục
độc lập** trong `WAVE-SEQUENCE.md`:

| Trục | Giá trị | Ý nghĩa |
|---|---|---|
| `wave_class` | `slice` | Lát mỏng (~1 ngày), test cấp 1 |
| | `integration` | Lát đầy đủ (~3–5 ngày), test toàn diện |
| `wave_strategy` | `vertical` | Cắt dọc 1 EPIC (BE + FE cùng chủ đề) |
| | `horizontal-be` | Chỉ backend (≤3 boundary), chưa UI |
| | `horizontal-fe` | Chỉ frontend (≤3 experience), consume contract wave trước |

**Ràng buộc cứng:** ≤3 target/layer/wave (context budget ≤80KB/agent); wave `horizontal-*`
phải đúng **một** tầng.

### 3.2 Dual-Track A/B — 7 wave / 7 tuần / team 10

CARxRENTAL chốt (ADR-D7-001) chiến lược **hybrid**:
- **Dual-Track Parallel** (khung chiến lược): Track A Fleet SaaS ∥ Track B Sàn Marketplace
- **MoSCoW Timeboxed** (cơ chế quản scope) — có WON'T-list công khai chống scope-creep
- **Walking Skeleton** (kỹ thuật de-risk): W3 đóng escrow saga E2E golden-path sớm

**Team:** 10 người (0.5 PO, 0.5 BA, 1 Dev Lead, 3 BE, 1.5 FE, 1 Mobile, 2 QC, 1 Design).
**Cadence:** 7 wave × ~1 tuần = 7 tuần tổng.

**Nối duy nhất tại seam:** contract `fleet-core-availability-allotment-v1` + data `fleet-core-
allotment-pool-v1` (cả 2 ACTIVE từ D4). **AllotmentPool SoT trong fleet-core là điểm nối duy
nhất giữa 2 track — KHÔNG tạo seam khác** (ADR-CR-036: escrow saga + POST /availability/hold
atomic idempotent + compensation release-hold).

**WON'T-THIS-TIME** (công khai, chống scope-creep — ADR-CR-049):
- P2P / B2D Marketplace (Phase 2+)
- TrustScore tổng hợp CAP-33 — Phase 2
- VehiclePnL / P&L per xe CAP-59 — cần data tích lũy Phase 2
- Chấm công tài xế CAP-56 — Phase 2
- Quản lý bảo dưỡng xe CAP-06 — Phase 2
- ACN / DriverPlus / OnePay integration **thật** — Phase 1 mock/sandbox (cross-repo CR chờ ACN thật)

**Wave inventory:**

| Wave | Tuần | Class | Strategy | Track | Target chính | Mục tiêu DEMO |
|---|---|---|---|---|---|---|
| **W1** | T1 | slice | horizontal-be | Foundation (chung) | fleet-core, tenant-management, identity-verification | Milestone nội bộ: `POST /availability/hold` atomic pass race-test (no oversell), seam contract fixture xanh, KYB fleet gate onboard, deactivate tenant. |
| **W2** | T2 | slice | vertical | **A — Fleet SaaS** | fleet-core, fleet-finance, fleet-console | **Milestone bảo hiểm** (sign-off Delivery): Fleet vận hành standalone (InternalBooking + InternalReceipt + dashboard), KHÔNG chạm payment. Ship được kể cả khi Track B trượt. |
| **W3** | T3 | integration | vertical | **B — Sàn (Walking Skeleton)** | booking, payment, handover | **DEMO E2E quan trọng nhất**: 1 đơn tiền chạy hết vòng escrow (PaymentRequested→capture→Settled→Confirmed→handover→FinalSettlement→payout→Closed) + 1 compensation release-hold. Đóng rủi ro tồn vong sớm. |
| **W4** | T4 | slice | vertical | A + B | driver-management, payment, marketplace (+ driver-app skeleton) | Milestone nội bộ: chauffeur booking variant E2E, claim flow, bảo hiểm per-trip offline P1, subscription billing manual P1, marketplace listing/search với DriverPlus mock, driver-app trên staging. |
| **W5** | T5 | slice | horizontal-fe | FE (2 web) | fleet-console, platform-admin | Milestone UI: fleet-console allotment reconcile + platform-admin KYB/policy/dispute; FE consume 100% contract ACTIVE. |
| **W6** | T6 | integration | vertical | **SEAM (A∥B)** | notification, reputation, platform-ops | **DEMO dual-path**: Track A internal + Track B marketplace chạy ĐỒNG THỜI trên AllotmentPool chung, không oversell (held_count invariant xanh ở tải đồng thời — ADR-CR-036); GMV reconcile manual P1, in-app messaging. |
| **W7** | T7 | integration | vertical | **Hardening** | (không target mới) | **DEMO final + go/no-go**: load-test hold race (10× concurrent no-oversell), idempotency OnePay/DriverPlus retry, UAT Track A+B sign-off Delivery + Business Authority. |

> 📌 Test isolation gộp vào wave chính (W2 milestone bảo hiểm Track A, W3 escrow E2E, W6 dual-
> path no-oversell, W7 hardening go/no-go) — không có wave test riêng.

**Nhịp demo:** 2 demo external quyết định (**W3 escrow E2E de-risk**, **W6 dual-path no-oversell**)
+ 1 go/no-go cuối (**W7**). W1/W2/W4/W5 là milestone nội bộ sign-off Authority, không demo external.
W2 = **milestone bảo hiểm** — có sản phẩm Fleet SaaS ship được kể cả khi Track B trượt.

> ⚠️ **Cảnh báo Authority ghi ADR-D7-001:** 3 BE + 12 backend polyglot + external chỉ mock =
> **pilot slice không phải MVP đầy đủ**. Cần ~11-12 tuần cho cả 2 góc production-ready. 7 tuần
> là commitment ship 2 walking skeleton + bảo hiểm Track A.

### 3.3 Một Wave đi qua trạm nào (recap)

```
SPECS mở wave  →  DOMAIN (BA/PO viết FEAT+BR)
                       ↓
   ARCHITECT (HLD+ADR) ∥ UIUX (design-system+tokens)  ← song song
                       ↓
   SPECS chốt contract (ratify + ký hash)  ← CONTRACT_READY_GATE
                       ↓
       /sync-design → snapshot design xuống code repo
                       ↓
   BOUNDARIES (BE) ∥ EXPERIENCES (FE)  ← song song, cùng contract
                       ↓
                    TESTING (QA thật)  → phán quyết ship
                       ↓
                   SPECS đóng wave  → /sign-qc
```

Chi tiết mỗi trạm ở §6–§10 (cẩm nang role).

---

## §4. Kiến trúc CARxRENTAL

### 4.1 12 boundary backend — chia 2 tier theo stack

| Tier | Stack | Boundary | Mission ngắn |
|---|---|---|---|
| **Hot-path** (5) | **Go 1.26 + Echo** | `payment` | Xử lý giao dịch, settlement, escrow |
| *(latency-sensitive, throughput cao)* | | `booking` | Quản lý booking marketplace + fleet |
| | | `marketplace` | Match renter ↔ fleet listing + pricing |
| | | `notification` | Push / email / SMS realtime |
| | | `handover` | Bàn giao xe tại Service Stations |
| **CRUD-heavy** (7) | **Node + NestJS** | `fleet-core` | Fleet + vehicle catalog |
| *(schema-heavy, dev velocity)* | | `fleet-finance` | Kế toán fleet, doanh thu, chi phí |
| | | `driver-management` | Chauffeur/driver profile + assignment |
| | | `tenant-management` | Multi-tenant management |
| | | `identity-verification` | KYC, license verification |
| | | `reputation` | Rating, review, trust score |
| | | `platform-ops` | Platform-side ops (admin, dispute) |

> 📌 **Vì sao polyglot?** Xem **CR-021**: hot-path cần concurrency + throughput (Go tối ưu),
> CRUD-heavy cần dev velocity + ecosystem TypeORM/NestJS. Chấp nhận chi phí learning curve 2
> stack đổi lại phù hợp workload từng boundary.

### 4.2 3 BFF — NestJS + @nestjs/apollo GraphQL

BFF pattern (ADR-D3-004 + ADR-CR-024): FE không gọi thẳng backend boundary, đi qua BFF —
BFF aggregate + shape response, giảm N+1.

| BFF | Kênh phục vụ | Aggregate backend |
|---|---|---|
| `bff-fleet` | fleet-console (web) + driver-app (mobile) | fleet-core, fleet-finance, driver-management, tenant-management, booking, handover, identity-verification, reputation, notification |
| `bff-marketplace` | DriverPlus external channel (inbound RentalRequest + outbound listing detail + status push) | marketplace, booking, payment, handover, identity-verification, reputation, notification |
| `bff-admin` | platform-admin (web) | platform-ops, tenant-management, payment, booking, fleet-core, identity-verification, reputation, driver-management |

> 📌 `bff-marketplace` là **API gateway 2 chiều với DriverPlus** (INV-001): (a) inbound rental
> request từ DriverPlus → relay xuống booking domain (channel=acn), (b) outbound aggregate
> listing detail + status push cho DriverPlus query. Không có FE của CARxRENTAL đi qua BFF này.

### 4.3 3 experience

CARxRENTAL không tự build app/web cho Consumer — Consumer touch point là **DriverPlus**
(external app, INV-001). Do đó chỉ có 3 experience nội bộ:

| Loại | Experience | Stack | Persona phục vụ |
|---|---|---|---|
| **Web** (2) | `fleet-console` | React 18 + Vite + TypeScript + **Tailwind + shadcn/ui** (CR-025) + TanStack Router + TanStack Query 5 + Zustand + React Hook Form + Zod | P-FLEETOWNER, P-FLEETACCOUNTANT |
| | `platform-admin` | ↑ (cùng stack) | P-PLATFORMOPS, P-CS, P-TENANTOPS, P-PLATFORMFINANCE |
| **Mobile** (1) | `driver-app` | **Flutter + Dart + flutter_bloc (Bloc + Cubit)** (CR-026) — Material 3 | P-DRIVER |

> 📌 **Vì sao Flutter?** 1 codebase iOS + Android, tận dụng team đã có kinh nghiệm, tránh maintain
> 2 native codebase riêng. State management: `flutter_bloc` — testable, tách UI/business logic rõ.

### 4.4 Stack authoritative

Stack thực đang dùng, kèm ADR/CR chốt quyết định (xem `Execution/tracking/decisions.md` cho lý do đầy đủ):

| Layer | Choice | Ref |
|---|---|---|
| Backend language | **Polyglot** Go 1.26 (5 hot-path) + NestJS (7 CRUD-heavy) | ADR-D3-001, ADR-CR-031 |
| Go HTTP framework | **Echo v4** (`github.com/labstack/echo/v4`) | ADR-CR-023 |
| Backend ORM | **GORM** (Go) + **TypeORM** (Node) | ADR-CR-018 |
| BFF stack | **3 × NestJS + `@nestjs/apollo`** GraphQL | ADR-D3-004, ADR-CR-024 |
| Frontend web | React 18 + Vite + TS + **Tailwind + shadcn/ui unified** cho cả 2 web + TanStack Router / Query 5 + Zustand + RHF + Zod | ADR-D3-002, ADR-CR-025 |
| Mobile | **Flutter + Dart + `flutter_bloc` (Bloc + Cubit)** + Material 3 | ADR-D3-002, ADR-CR-026 |
| Data / Broker | Postgres RDS multi-tenant **pooled + `tenant_id` row-level** + Kafka **MSK** | ADR-D3-003 |
| Observability | **OTel + OTel Collector → Elastic** (Elasticsearch + Kibana + APM) — metrics OTLP native | ADR-CR-027, ADR-CR-030 |
| Identity | **Keycloak** self-host EKS | ADR-CR-020 |
| Secret store | **AWS Secrets Manager** (RDS auto-rotation) | ADR-CR-021 |
| SAST | **SonarQube unified** | ADR-CR-018 |
| Dep-audit | Go `govulncheck` + Node `npm audit` + Dependabot | ADR-CR-019 |
| Log shipper | **OpenTelemetry Collector unified** | ADR-CR-022 |
| Infra | AWS **EKS** + ArgoCD/Helm + RDS + MSK + ElastiCache + S3 | ADR-D3-003 |

---

## §5. Contracts (60 file ACTIVE)

### 5.1 4 loại contract

| Loại | Số file | Nội dung | Ai ký |
|---|---|---|---|
| **api** | 15 | REST/GraphQL BFF ↔ backend; BFF ↔ FE; external channel (DriverPlus, ACN) | FE + BE + external consumer |
| **event** | 26 | Kafka event cross-boundary (publish/subscribe) — canonical event ADR-CR-002..016 | Producer + subscriber |
| **ui** | 3 | Design token + component contract UIUX → FE | FE consumer |
| **data** | 6 | Schema read-model + analytics + allotment pool | Analytics + downstream consumer |

**Trạng thái sau D4/D5:** ADR-CR-039 flip **58 contract DRAFT → ACTIVE** + ký **130 consumer
signature** hash-verify PASS (chỉ giữ 1 DRAFT: `fleet-finance-vehicle-pnl-v1` — Phase 2). Chi
tiết ownership + producer/consumer: `CONTRACT-MAP.md`.

### 5.2 Contract-First workflow

**FE code chỉ dựa contract signed** → không cần biết BE đã implement chưa. Điều này cho phép
BE + FE làm **song song** không chờ nhau:

```
D4: contract-steward ký contract  →  hash SHA256 lưu vào tracking/contract-signatures.json
     ↓
FE dev: dùng mock generate từ contract  →  code UI + integration
BE dev: implement contract  →  test-contract check khớp signature
     ↓
QC: test integration khi cả 2 xong
```

Nếu implementation lệch contract đã ký → **KHÔNG patch code cho khớp**. Mở CR về SPECS, ký lại
contract, `/sync-design` lại. Nguyên tắc: contract là source-of-truth; code chảy theo contract,
không ngược lại.

### 5.3 Signature hash + drift protection

- Mỗi contract có **SHA256 hash** lưu trong `tracking/contract-signatures.json` mỗi consumer.
- Hook `contract-hash-check` (sẽ enable ở SPECS) re-hash mỗi lần đụng source → hash lệch =
  **block** với failure mode `FM-CONTRACT-DRIFT`.
- Sửa contract → **bump version** + **ký lại** cho tất cả consumer.
- Tool hiện có: `scripts/contract-sign.py verify <consumer>` — chạy manual verify signature.

> ⚠️ **CARxRENTAL hiện chưa enable `contract-hash-check` PreToolUse hook** (audit gap). Sẽ
> enable ở SPECS sau bootstrap — trước đó, drift chỉ được phát hiện thủ công qua review +
> `contract-sign.py verify`.

---

# ═══════════ PHẦN B — CẨM NANG 5 ROLE ═══════════

> 📖 **Cấu trúc chung cho mỗi role:**
> 1. Bạn ở trạm nào (repo + phase)
> 2. Ngày đầu tiên đọc gì (top 5–7 file)
> 3. Stack + tool + agent bạn dùng
> 4. Vòng đời 1 task điển hình (state machine LOCAL)
> 5. `owned_paths` — được sửa gì, cấm gì
> 6. Cổng ký duyệt (exit gate)
> 7. Khi lệch chuẩn → làm gì
> 8. ✅ Checklist "người mới cần nắm"
> 9. FAQ + pitfall

---

## §6. DEV BACKEND

### 🎯 6.1 Trạm

Bạn làm ở **`-BOUNDARIES`** — *repo "vỏ"* chứa **N boundary repo con**, mỗi boundary là 1 git
repo riêng theo cùng khuôn.

⚠️ **Repo `-BOUNDARIES` CHƯA bootstrap** — sẽ tạo ở bước execution. Trong lúc chờ, đọc
CHARTER + contract của boundary bạn sẽ giữ để sẵn sàng.

### 📚 6.2 Ngày đầu — đọc theo thứ tự

1. `CLAUDE.md` của boundary được giao (identity + non-negotiables)
2. `design/MANIFEST.md` — scope wave hiện tại + exit criteria + allowlist contract + lệnh build/lint/test
3. `design/CHARTER.md` — mission + owned data + owned paths + capabilities
4. `design/product/feats/FEAT-*.md` — AC nghiệp vụ (viết bằng ngôn ngữ thường)
5. `design/contracts/api/*.md` + `design/contracts/event/*.md` — đầu nối bạn phải implement
6. `_shared/coding-standards/backend-golang.md` (nếu Go) hoặc `backend-nodejs.md` (nếu Node) + `_shared/security-policy.md` + `_shared/observability-policy.md`
7. `_aggregated/PRD.md` + `_aggregated/SYSTEM-ARCHITECTURE.md` + `_aggregated/TECHSTACK.md` — context tổng

### 🧰 6.3 Stack theo boundary

| Nếu boundary bạn giữ là… | Stack |
|---|---|
| 5 hot-path (payment / booking / marketplace / notification / handover) | **Go 1.26 + Echo v4** (ADR-CR-023, CR-031) + **GORM** (CR-018) |
| 7 CRUD-heavy (fleet-core / fleet-finance / driver-management / tenant-management / identity-verification / reputation / platform-ops) | **NodeJS + NestJS + TypeORM** (CR-018) |

**Cross-cutting (mọi boundary):**
- **Database:** PostgreSQL RDS **multi-tenant pooled + `tenant_id` row-level** (ADR-D3-003). Mỗi
  boundary có schema riêng, KHÔNG cross-boundary query trực tiếp — đi qua contract event/api.
- **Event bus:** Kafka MSK. Canonical event tên chốt ADR-CR-002..016 (KHÔNG tự đặt tên mới).
- **Observability:** **OTel Collector → Elastic** (Elasticsearch + Kibana + APM). Metrics OTLP
  native, không qua Metricbeat (ADR-CR-030).
- **Identity:** Keycloak self-host EKS (ADR-CR-020) — JWT + realm-per-tenant.
- **Secret:** AWS Secrets Manager với RDS auto-rotation (ADR-CR-021) — External Secrets Operator
  sync xuống K8s. KHÔNG hardcode secret.
- **SAST + dep-audit:** SonarQube + `govulncheck` (Go) / `npm audit` (Node) + Dependabot (CR-019).

**6 agent chạy trong service repo của bạn:**
`dev-backend`, `fix-backend`, `review-backend`, `test-unit`, `test-contract`, `test-integration`.

Gọi qua slash command: `/spawn-dev`, `/spawn-fix <bug>`, `/spawn-review`, `/spawn-test-*`.

### 🔁 6.4 Vòng đời task (state machine LOCAL)

```
IDLE  →  IMPLEMENTING  →  READY_FOR_REVIEW
              ↑                  ↓
              └──── FIX_BUGS ←───┘
                    ↓
              READY_FOR_TEST  →  DONE
                    ↓
                 (BLOCKED — escape hatch)
```

Mỗi transition qua `/stage-transition` — hook `state-guard.py` check condition.

### 🔒 6.5 owned_paths

| Được sửa | Cấm sửa (hook chặn) |
|---|---|
| `src/**` | `design/**` — chỉ đọc, snapshot từ SPECS |
| `tests/**` | `contracts/**` — chỉ đọc, ký ở SPECS |
| `requirements/**` | `_shared/**` — chỉ đọc |
| `.knowledge-graph.yml` | `tracking/contract-signatures.json` — chỉ system update |

### 🚪 6.6 Cổng dev-ready

Cổng `/wave-gate-dev-ready` yêu cầu **tất cả**:

- ✅ Build green
- ✅ Lint green (theo `_shared/coding-standards/backend-{golang,nodejs}.md`)
- ✅ Test unit + contract + integration green
- ✅ Coverage đạt ngưỡng (chốt ở MANIFEST)
- ✅ Mọi FEAT AC pass
- ✅ 0 test rỗng (không stub)
- ✅ Chữ ký contract khớp (`contract-hash-check`)
- ✅ Decision-log đủ: mỗi commit đáng kể có `# DECISION-REF: ADR-xxx | BR-yyy | CONTRACT-zzz`
- ✅ Knowledge-graph cập nhật (`.knowledge-graph.yml`)
- ✅ Không hardcode secret (scan bởi `secret-scan.py`)

### 🚨 6.7 Khi lệch chuẩn

| Tình huống | Xử lý |
|---|---|
| Contract signature không khớp (drift) | **Mở CR về SPECS** — ký lại contract, `/sync-design` lại. **KHÔNG patch code cho khớp.** |
| AC không code được (nghiệp vụ vô lý) | Blocker → escalate BA + Business Authority |
| Cần cross-boundary query | Blocker → escalate Architecture Authority → CR MAJOR đổi contract event |
| Test integration fail vì boundary khác chưa xong | Đó là chuyện bình thường của horizontal-be wave — chờ dev-ready wave sau |

### ✅ 6.8 Checklist người mới cần nắm

- [ ] Biết boundary bạn được giao thuộc **tier hot-path (Go)** hay **CRUD-heavy (NestJS)** và **lý do** technical trade-off
- [ ] Mở CHARTER, chỉ ra: **owned data**, **owned paths**, **capabilities** boundary bạn giữ
- [ ] Mở 1 contract `api/`, đọc endpoint + request + response + error format
- [ ] Hiểu vì sao **KHÔNG được sửa `design/**`** (bản chụp read-only từ SPECS)
- [ ] Biết drift → **mở CR**, không patch code
- [ ] Vẽ được state machine LOCAL của service (`IDLE → … → DONE` + BLOCKED)
- [ ] Biết decision-log: mỗi commit đáng kể phải có `DECISION-REF`
- [ ] Biết cổng dev-ready đòi 10 điều kiện (§6.6)

### ❓ 6.9 FAQ

- **"Boundary tôi giữ dùng Go — tôi chỉ biết Node, học được không?"** → Được. Go syntax nhẹ,
  ~2 tuần onboarding. Coding standard `_shared/coding-standards/backend-golang.md` có
  playbook + example. Ngược lại NestJS quen thì fast.
- **"Kafka event schema đổi — tôi update trực tiếp được không?"** → **KHÔNG.** Event
  schema là **contract event**, phải mở CR + bump version + subscriber ký lại. Breaking change
  event = CR **MAJOR** (§13). Tên event canonical đã chốt ADR-CR-002..016 — grep decisions.md
  trước khi đặt tên mới.
- **"Postgres multi-tenant — pooled model có phải RLS không?"** → CARxRENTAL Phase 1 chốt
  **pooled shared DB + `tenant_id` row-level** (ADR-D3-003), không hẳn Postgres RLS policy
  formal. Mọi query BẮT BUỘC filter `tenant_id` — enforce ở ORM layer (GORM scope / TypeORM
  subscriber). Bypass = data leak → CR **CRITICAL** + Bảo mật ký.
- **"Boundary tôi giữ đụng đến AllotmentPool — sửa contract fleet-core được không?"** → KHÔNG.
  AllotmentPool SoT là **seam duy nhất** giữa 2 track (ADR-CR-036). Đổi contract seam = CR
  **MAJOR** + Architecture ký + rebuild race-test invariant.

---

## §7. DEV FRONTEND

### 🎯 7.1 Trạm

Bạn làm ở **`-EXPERIENCES`** — repo "vỏ" chứa 3 experience nội bộ: 2 web (`fleet-console`,
`platform-admin`) + 1 mobile (`driver-app`). Consumer đi qua DriverPlus external app, CARxRENTAL
không build UI cho Consumer.

⚠️ **Repo `-EXPERIENCES` CHƯA tồn tại** — sẽ bootstrap sau bước execution.

### 📚 7.2 Ngày đầu — đọc theo thứ tự

1. `CLAUDE.md` của experience được giao
2. `design/MANIFEST.md` — scope wave + exit criteria + design-system version
3. `design/CHARTER.md` — mission + owned paths + persona phục vụ
4. `design/product/feats/FEAT-*.md` — AC + persona pain-point
5. `design/product/wireframes/*.md` — mockup + UX flow (BA vẽ)
6. `design/contracts/api/*.md` — endpoint BFF (KHÔNG phải boundary trực tiếp)
7. `design/contracts/ui/*.md` — component API + design token contract
8. `design/design-systems/tokens.json` + `design/design-systems/components/*.md`
9. `_shared/coding-standards/frontend-web.md` (nếu web) hoặc `frontend-mobile.md` (nếu mobile)

### 🧰 7.3 Stack theo experience

| Nếu experience bạn giữ là… | Stack |
|---|---|
| Web (`fleet-console` / `platform-admin`) | **React 18 + Vite + TypeScript + Tailwind + shadcn/ui unified (CR-025) + TanStack Router + TanStack Query 5 + Zustand + React Hook Form + Zod** |
| Mobile (`driver-app`) | **Flutter + Dart + `flutter_bloc` (Bloc + Cubit) (CR-026) + Material 3** |

**BFF routing (nhớ ai đi qua BFF nào):**
- `fleet-console` (web) → `bff-fleet` (GraphQL)
- `driver-app` (mobile) → `bff-fleet` (GraphQL — CÙNG BFF với fleet-console)
- `platform-admin` (web) → `bff-admin` (GraphQL)
- Không có FE nào đi qua `bff-marketplace` — nó là API surface cho DriverPlus external.

**BFF pattern:** FE **CHỈ gọi BFF** (GraphQL), **KHÔNG gọi thẳng backend boundary**. BFF
aggregate + shape response cho FE. Lý do: giảm N+1, tách concern (BE thay đổi internal API
không đụng FE).

**Design system:** consume `tokens.json` — codegen thành CSS variables (web) hoặc Dart theme
(mobile). **KHÔNG hardcode màu / spacing / font** trong component. Đổi token = đổi contract UI
→ CR.

**6 agent:** `dev`, `fix`, `review`, `test-unit`, `test-component`, `test-contract`.

### 🔁 7.4 Vòng đời

Giống BE (§6.4): `IDLE → IMPLEMENTING → READY_FOR_REVIEW → (FIX_BUGS ↔ IMPLEMENTING)* → READY_FOR_TEST → DONE`.

### 🔒 7.5 owned_paths

| Được sửa | Cấm sửa |
|---|---|
| `src/**` | `design/**` (bản chụp) |
| `tests/**` | `contracts/**` |
| `requirements/**` | `_shared/**` |
| `.knowledge-graph.yml` | `design-systems/**` (chỉ UIUX ghi ở source) |

### 🚪 7.6 Cổng dev-ready (thêm so với BE)

Ngoài 10 điều kiện của BE, FE cần thêm:
- ✅ **Playwright component test** green (web)
- ✅ **axe-core accessibility** score đạt ngưỡng (web)
- ✅ **Design token consume verified** — không hardcode màu/spacing
- ✅ **Mobile:** flutter test + widget test + integration test green

### 🚨 7.7 Khi lệch chuẩn

| Tình huống | Xử lý |
|---|---|
| Design token thiếu (thiếu màu / spacing scale) | Blocker → escalate UIUX → CR MODERATE bump ui contract |
| BFF endpoint chưa có mock (BE chưa xong) | Generate mock từ contract `api/*.md` — dev tiếp không chờ BE |
| Design system component thiếu variant cần dùng | KHÔNG tự tạo variant local. CR về UIUX. |
| AC yêu cầu behavior không có trong contract | Blocker → BA + Architecture |

### ✅ 7.8 Checklist người mới cần nắm

- [ ] Biết experience bạn giữ dùng **React (web)** hay **Flutter (mobile)**
- [ ] Hiểu vì sao chỉ gọi **BFF**, không gọi thẳng boundary
- [ ] Mở `design/design-systems/tokens.json` và biết cách consume (CSS vars web / Dart theme mobile)
- [ ] Hiểu **contract-first**: mock BFF từ contract → dev không cần chờ BE
- [ ] Biết drift → **CR**, không hardcode override token
- [ ] Biết cổng dev-ready FE cần thêm accessibility + component test

### ❓ 7.9 FAQ

- **"Tôi muốn dùng styled-components / AntD thay Tailwind + shadcn/ui — được không?"** →
  **KHÔNG.** Stack đã chốt: ADR-D3-002 + **ADR-CR-025** (unified Tailwind + shadcn/ui cho cả
  3 web). Đổi = CR **MAJOR** + Architecture ký + rework 2 experience web.
- **"BFF GraphQL bị over-fetch — tôi kêu BE thu gọn query được không?"** → Không kêu trực
  tiếp. Mở CR MODERATE về SPECS → contract api sửa → BE update BFF.
- **"Mobile app share code với web được không?"** → Không (Flutter Dart vs React TS). Chỉ
  share **contract** + **design token**.
- **"Tôi cần build UI cho Consumer thuê xe — làm ở đâu?"** → **KHÔNG làm.** Consumer touch
  point là **DriverPlus** (INV-001, ADR-CR-043). CARxRENTAL không có Consumer app / web.
  Nếu có yêu cầu → CR **CRITICAL** vì phá INV-001.

---

## §8. QC (Quality Control)

### 🎯 8.1 Trạm

Bạn làm ở **`-TESTING`** repo. Nhiệm vụ: chạy test **THẬT**, không mock, phán quyết ship hay không.

⚠️ Repo CHƯA bootstrap — sẽ tạo ở bước execution.

### 📚 8.2 Ngày đầu — đọc theo thứ tự

1. `TESTING/CLAUDE.md` — role routing + non-negotiables
2. `registry/test-cases.yml` — **pool test case tích lũy** (Jaccard dedup ≥0.7 → REUSE)
3. `waves/W<current>/` — plan test cho wave đang chạy
4. `product/` — đọc FEAT + AC (biết "đúng là gì")
5. `contracts/` — biết đầu nối (test-contract layer)
6. `bugs/` — lịch sử bug đã report

### 🧰 8.3 Tool + agent

| Tool | Dùng cho |
|---|---|
| **curl** | Test API contract (BE endpoint) |
| **Playwright** | E2E web + component test |
| **k6** | Load test (hot-path boundary) |
| **axe-core** | Accessibility (web) |
| **Flutter driver** | E2E mobile (driver-app) |

**3 agent QC:**
- `qa-author` — viết test case, dedup Jaccard ≥0.7 vào registry
- `qa-executor` — chạy **THẬT** (curl/Playwright/k6/axe), log stdout/stderr/screenshot
- `qa-translator` — re-map test case khi AC đổi

### 🚫 8.4 Anti-fake invariant

**Service down → mark BLOCKED, KHÔNG giả PASS.** Hook `check_connectivity.py` chạy trước
mỗi test suite:
- HTTP 5xx / connection refused / timeout → **BLOCKED**
- Chỉ đếm PASS/FAIL khi service **thật sự lên**

Hook `capture-evidence` lưu:
- stdout + stderr
- Playwright screenshot + video
- curl request + response headers
- k6 metrics HDR
→ nộp làm bằng chứng cho `/wave-signoff`.

### 🐞 8.5 Bug — routing rõ ràng

Mỗi bug BẮT BUỘC khai:

| Field | Ví dụ |
|---|---|
| `layer` | backend / frontend / integration / infra / config |
| `boundaries` (nếu backend) | payment, booking |
| `experiences` (nếu frontend) | fleet-console |
| `severity` | P1 (block ship) / P2 (major) / P3 (minor) / P4 (cosmetic) |
| `linked_feat` | FEAT-0421 |
| `linked_br` | BR-0089 |
| `found_in_tc` | TC-1204 |

→ dev đúng đội `/sync-bugs-to-specs` pull đúng bug.

### ⚖️ 8.6 Ba phán quyết `/wave-signoff`

| Verdict | Điều kiện | Hệ quả |
|---|---|---|
| **APPROVED** | 0 P1/P2 OPEN + coverage đạt ngưỡng | ✅ Ship-ready |
| **CONDITIONAL** | Còn P3 backlog / coverage thiếu nhẹ | ⚠️ Ship kèm điều kiện (verify wave sau) |
| **REJECTED** | Còn P1 OPEN / fail exit criteria | ✗ Chặn ship → quay lại fix |

### 🔒 8.7 CARxRENTAL — QA BẮT BUỘC ≠ Business

Nguyen Ha Anh giữ Business role → **KHÔNG được kiêm QA sign-off.** QA lead: `qa@cardoctor.vn`.
Hook `signoff-guard.py` chặn nếu email QA == email Business trong `AUTHORITIES.yaml`.

### 🎯 8.8 Test moment đặc biệt CARxRENTAL

Test isolation gộp vào wave chính (không có wave test riêng):

| Wave | Vị trí | Nhiệm vụ QC |
|---|---|---|
| **W2** | Cuối Track A slice (bảo hiểm) | Test regression Fleet SaaS standalone — verify Fleet vận hành nội bộ đủ để ship kể cả khi Track B trượt |
| **W3** | Walking Skeleton escrow | **E2E golden-path escrow** (PaymentRequested→Settled→Confirmed→handover→FinalSettlement→payout→Closed) + 1 compensation release-hold — bắt buộc pass để tiếp W4 |
| **W6** | SEAM dual-path | **Load test AllotmentPool no-oversell** — Track A ∥ Track B chạy đồng thời trên pool chung, `held_count invariant` xanh (ADR-CR-036) |
| **W7** | Hardening (go/no-go) | 10× concurrent hold race, idempotency OnePay/DriverPlus retry, UAT Track A+B sign-off Delivery + Business Authority |

### 🚨 8.9 Khi lệch chuẩn

| Tình huống | Xử lý |
|---|---|
| AC không test được (thiếu điều kiện đo) | Log bug **P2 layer=product** → BA update FEAT/AC → CR MINOR |
| Test contract fail nhưng service PASS | Có nghĩa contract lệch implementation — **CR** về SPECS |
| Phát hiện bug security P1 giữa wave | `/blocker-raise CRITICAL` → SM route Architecture + Bảo mật |

### ✅ 8.10 Checklist người mới cần nắm

- [ ] Hiểu "test THẬT" (anti-fake): service chết → **BLOCKED**, không giả PASS
- [ ] Phân biệt 3 phán quyết: **APPROVED / CONDITIONAL / REJECTED**
- [ ] Hiểu vì sao **QA BẮT BUỘC ≠ Business** (không vừa đá bóng vừa thổi còi)
- [ ] Biết bug phải có: `severity + layer + boundaries + linked_feat + linked_br + found_in_tc`
- [ ] Biết **Jaccard dedup ≥0.7** → REUSE test case cũ, không viết trùng
- [ ] Biết `registry/` là pool tích lũy → hồi quy tự động ở wave sau
- [ ] Biết 4 test moment đặc biệt: W2 (Fleet bảo hiểm), W3 (escrow E2E), W6 (dual-path no-oversell), W7 (hardening go/no-go)

### ❓ 8.11 FAQ

- **"Khi nào escalate CR vs chỉ log bug?"** → Log bug: implementation lệch spec. CR: spec
  vô lý / không test được / cần đổi behavior.
- **"CONDITIONAL sign-off — verify khi nào?"** → Ghi vào `waves/W<next>/carry-over.yml`,
  hook `qa-carryover.py` chặn `/wave-signoff` wave sau nếu carry-over chưa clear.
- **"Bug security P1 phát hiện sau ship — ai xử?"** → Escalate CRITICAL, Bảo mật ký hotfix
  wave (out-of-sequence).

---

## §9. SCRUM MASTER

> 📌 **Ghi chú quan trọng:** ADLC v4.5 **không có role "Scrum Master" nguyên bản** — role gần
> nhất là **Delivery**. Trong CARxRENTAL, Scrum Master = **Delivery + observability +
> facilitation**. Bạn:
> - **Không tự ký duyệt kỹ thuật** (đó là Architecture / Bảo mật)
> - **Không tự ký duyệt ship** (đó là QA)
> - **Ký:** mở/đóng wave, cadence, unblock cross-team

### 🎯 9.1 Trạm

**Chính:** `-SPECS` (băng chuyền trung tâm, giữ STATE machine của wave).
**Phụ:** `-ORCHESTRATOR` (dashboard read-only, FastAPI + React) — quan sát 8 trạm real-time.

⚠️ Cả 2 CHƯA bootstrap. Trong lúc chờ, đọc `Execution/STATE.json` + `plan/WAVE-SEQUENCE.md`
ở repo DISCOVERY.

### 📚 9.2 Ngày đầu — đọc theo thứ tự

1. `_aggregated/ROADMAP.md` — bức tranh 7 wave / 7 tuần
2. `plan/WAVE-SEQUENCE.md` — chi tiết mỗi wave: scope + target + exit criteria + cadence + demo goal
3. `SPECS/Execution/STATE.json` (khi có) — trạng thái wave hiện tại
4. **Orchestrator dashboard** `http://127.0.0.1:5173` (cần `ADLC_ADLC_ROOT` override)
5. `Execution/tracking/decisions.md` — 58 ADR/CR đã chốt (precedent library)
6. Grep decisions.md cho pattern `ADR-CR-*` hoặc `ADR-D3-*` / `ADR-D7-*` — chưa có file `CR-LOG.md` riêng
7. `boundaries/*/CHARTER.md` (12 backend + 3 BFF) + `web-experiences/*/CHARTER.md` (2) + `mobile-experiences/*/CHARTER.md` (1) — **18 target** dạng one-liner để biết đội nào phụ trách gì

### 🧭 9.3 Trách nhiệm cốt lõi

| Việc | Slash command | Ghi chú |
|---|---|---|
| Mở wave | `/wave-start N` | Chuyển hệ thống sang trạng thái "đang làm wave N" |
| Giám sát tiến độ | Orchestrator dashboard + `SPECS/STATE.json` | Real-time WebSocket |
| Facilitate blocker | `/blocker-raise` + escalate đúng người | Cross-boundary → Architecture; scope → Business; ship → QA |
| Cadence | Theo WAVE-SEQUENCE — **7 wave × ~1 tuần = 7 tuần** (ADR-D7-001) | Team 10 (0.5 PO, 0.5 BA, 1 Dev Lead, 3 BE, 1.5 FE, 1 Mobile, 2 QC, 1 Design) |
| CR triage | `/cr-raise <severity>` | MODERATE/MAJOR/CRITICAL → route đúng người duyệt (§13) |
| Đóng wave | `/sign-qc` | Sau QA APPROVED |

### 🎯 9.4 Dual-Track A/B coordination

Track A (Fleet SaaS) và Track B (Sàn Marketplace) chạy **song song** từ W2/W3. Bạn phải:

- **W1 Foundation là root DAG chung** — fleet-core + tenant-management + identity-verification
  phải xanh trước khi W2/W3 tách. Slip W1 = slip cả 2 track.
- **W2 là milestone bảo hiểm Track A** — Fleet ship standalone được kể cả khi Track B trượt. Nếu
  W3 escrow saga fail → vẫn ship Fleet SaaS theo W2.
- **W3 Walking Skeleton là de-risk quyết định** — nếu escrow E2E golden-path không close trong
  W3, escalate CRITICAL ngay, cân nhắc pivot scope.
- **Seam AllotmentPool (fleet-core-availability-allotment-v1) là điểm nối duy nhất** giữa 2
  track. KHÔNG mở seam khác. W6 test dual-path no-oversell invariant.
- **W7 Hardening** = final go/no-go, không target mới, chỉ hardening + UAT.

### 🚨 9.5 Khi lệch chuẩn

| Tình huống | Xử lý |
|---|---|
| Wave bị slip cadence | Blocker MINOR → Delivery review; nếu 2 wave liên tiếp slip → escalate Architecture |
| Cross-boundary blocker (BE1 chờ BE2) | Route Architecture; consider re-sequence wave |
| QA REJECTED nhưng team muốn ship anyway | **KHÔNG bypass.** Đó là "vừa đá bóng vừa thổi còi". Escalate Business + Architecture cùng review |
| Team muốn skip W3 walking skeleton / W7 hardening | KHÔNG. Đó là non-negotiable — walking skeleton W3 de-risk + W7 go/no-go đã chốt ở ADR-D7-001 |
| Team muốn thêm target ngoài WON'T-list | KHÔNG. WON'T-list (ADR-CR-049) đã công khai chống scope-creep. Đổi = CR MAJOR + Delivery + Business ký |

### ✅ 9.6 Checklist người mới cần nắm

- [ ] Hiểu **Dual-Track A/B parallel + Walking Skeleton W3**: 2 track song song, W2 bảo hiểm Track A, W3 de-risk escrow
- [ ] Biết đọc `SPECS/Execution/STATE.json` ra "wave nào, stage nào"
- [ ] Phân biệt `/wave-start` (mở), `/wave-*-ready` (gate), `/wave-signoff` (QA quyết), `/sign-qc` (SM đóng)
- [ ] Biết **escalation path**: cross-boundary → Architecture; scope → Business; ship → QA
- [ ] Bật orchestrator dashboard hands-on (biết `ADLC_ADLC_ROOT` override)
- [ ] Biết CR level nào cần ai duyệt (§13)
- [ ] Hiểu **seam duy nhất = AllotmentPool** (ADR-CR-036) — W6 dual-path no-oversell invariant
- [ ] Biết WON'T-list (ADR-CR-049) — 4 Phase-2 cap cắt scope

### ❓ 9.7 FAQ

- **"Khác giữa `/wave-signoff` và `/sign-qc`?"** → `/wave-signoff` là **QA quyết** (APPROVED/
  CONDITIONAL/REJECTED). `/sign-qc` là **SM đóng wave** sau khi QA APPROVED. SM không tự
  signoff.
- **"Wave hiện tại slip 3 ngày — báo ai?"** → Log blocker MINOR + note trong wave-report;
  không cần escalate ngay. Slip 2 wave liên tiếp → escalate Delivery + Architecture.
- **"Tôi có được sửa `WAVE-SEQUENCE.md` không?"** → **KHÔNG.** WAVE-SEQUENCE là D7 output
  đông cứng. Đổi = CR **MAJOR** + Architecture + Delivery cùng ký.

---

## §10. BA (Business Analyst)

### 🎯 10.1 Trạm

Bạn làm ở **`-DOMAIN`** — repo nghiệp vụ thuần, **ngôn ngữ kinh doanh**, không lẫn kỹ thuật.

⚠️ Repo CHƯA tồn tại — sẽ bootstrap sau `DISCOVERED`.

### 📚 10.2 Ngày đầu — đọc theo thứ tự

1. `_aggregated/PRD.md` — bức tranh sản phẩm + 9 personas + 74 capabilities
2. `_discovery/persona-pool.md` — chi tiết 9 personas + 2 external actor + anti-personas
3. `_discovery/capability-map.md` — 74 capabilities gắn **Phase 1 / Phase 2 priority** + **candidate domain**
4. `_discovery/event-storming/ES-*.md` — **12 domain** (1 ES / boundary — xem BOUNDARY-MAP §1)
5. `_discovery/hypothesis-log.md` — Vision + Problem + Hypotheses + **5 Invariant INV-001..005** (chuỗi DriverPlus/CARxRENTAL/ACN — ADR-CR-040)
6. `boundaries/*/CHARTER.md` — mission + owned data (biết BR bạn viết chảy vào đâu)
7. `_shared/definitions/planning-rules.md` — quy tắc planning; chưa có `business-glossary.md` chuẩn

### ✍️ 10.3 Artifact bạn viết (DOMAIN repo)

| Artifact | Chủ | Chứa gì |
|---|---|---|
| **EPIC** | PO (BA hỗ trợ) | Tầm nhìn + hypothesis + success metric của một nhóm tính năng |
| **FEATURE** | PO (BA hỗ trợ) | **Acceptance criteria** ngôn ngữ thường + giá trị người dùng + priority |
| **JOURNEY** | PO (BA hỗ trợ) | Hành trình trải nghiệm + touchpoint + cảm xúc |
| **BUSINESS-RULE** | **BA** | Điều kiện + ngoại lệ + phạm vi dữ liệu |
| **PERSONA** | **BA** | Hồ sơ actor + mục tiêu + nỗi đau + bối cảnh |
| **WIREFRAME** | **BA** | Mockup low/mid-fi + luồng UX |

### 🚫 10.4 Kỷ luật ngôn ngữ nghiệp vụ

AC (acceptance criteria) viết dạng:

> "Khi khách hàng nhấp **[nút Xác nhận thuê]**, hệ thống kiểm tra điều kiện theo **BR-0089**;
> nếu đạt hiện form thanh toán, nếu không hiện lý do."

**CẤM lọt jargon kỹ thuật trong AC/BR/JOURNEY/EPIC:**
- endpoint, JWT, JSON, SQL, cookie, header, session
- tên component (`FleetCard`, `<BookingForm>`)
- tên layer (BFF, Redis, Kafka)
- tên table / microservice (`fleet_vehicles`, `payment-svc`)
- HTTP status code (`403`, `500`)

Hook `lint-frontmatter.py` **CHẶN `/sync-to-specs`** nếu phát hiện jargon → BA phải fix trước
khi sync.

### 🎯 10.5 Đặc thù CARxRENTAL

**2 sản phẩm (SaaS Fleet + Sàn) + 2 track — mỗi FEAT phải rõ track:**
```yaml
frontmatter:
  track: A  # A = Fleet SaaS (channel=internal); B = Sàn Marketplace (channel=marketplace|acn); cross = cả 2
  epic: EPIC-Fleet-Ops
  channel: internal  # hoặc marketplace, acn — quyết định luồng saga
```

**Multi-tenant (pooled model) — BR phải chỉ scope tenant:**
- `scope: single-tenant` — chỉ áp dụng trong 1 tenant (thường default)
- `scope: cross-tenant` — cross tenant (vd platform-ops action) — **cần Bảo mật ký**

**Chuỗi 3 bên DriverPlus/CARxRENTAL/ACN — BR bám INV-001..005:**
- Consumer là external actor (INV-001) — BR viết `Consumer thao tác` chỉ mô tả hành vi qua
  DriverPlus, không phải app CARxRENTAL.
- KYC Consumer thuộc ACN (INV-003) — BR CARxRENTAL không có bước "verify Consumer".
- KYB Fleet thuộc CARxRENTAL (INV-003) — do `identity-verification` verify fact + `platform-ops`
  ra quyết định approve (ADR-CR-011).

**Multi-persona interaction:** nhiều FEAT xuyên 2–3 persona. Ví dụ handover marketplace:
- `P-DRIVER` (giao xe cho renter tại ACN Service Station)
- Consumer external (nhận xe — thao tác qua DriverPlus)
- ACN Service Station Operator external (chứng kiến + xác minh vật lý)
- `P-PLATFORMOPS` (giám sát nếu có dispute)

BR phải chỉ rõ **actor** của mỗi bước + phân biệt persona nội bộ vs external actor.

### 🚨 10.6 Khi lệch chuẩn

| Tình huống | Xử lý |
|---|---|
| BR conflict với contract đã ký | **CR MODERATE** → Architecture + BA cùng review; contract có thể bump version |
| Persona thay đổi (thêm/sửa) | **CR MAJOR** → đụng D1, phải Business Authority ký |
| Chỉnh AC nhỏ, không đổi behavior | **CR MINOR** hoặc trực tiếp (nếu chỉ typo) |
| Capability mới không có trong 74 CAP | **CR MAJOR** → đụng D1 capability-map, Business + Architecture cùng ký |
| Anti-persona overlap (persona mới có traits anti-persona) | Blocker MAJOR → Business Authority review lại D0/D1 |
| Phá Invariant INV-001..005 (vd đề xuất CARxRENTAL tự KYC Consumer, hoặc build app Consumer) | **CR CRITICAL** → tất cả 5 Authority ký; đổi INV = đổi hiến pháp |

### ✅ 10.7 Checklist người mới cần nắm

- [ ] Phân biệt **EPIC / FEATURE / BUSINESS-RULE / PERSONA / JOURNEY** (5 loại artifact)
- [ ] Viết **AC không lọt jargon kỹ thuật** — biết hook `lint-frontmatter.py` chặn cái gì
- [ ] Đọc `capability-map.md`, biết FEAT bạn viết mapping capability nào + Phase 1 / Phase 2 priority
- [ ] Hiểu **9 personas + 2 external actor (DriverPlus, ACN) + anti-personas** (biết ai KHÔNG phục vụ để giữ scope)
- [ ] Hiểu **5 Invariant INV-001..005** — không được đề xuất phá chuỗi 3 bên
- [ ] Biết BR phải chỉ **`scope: single-tenant`** vs **`cross-tenant`**
- [ ] Biết đọc contract sau khi ký → verify AC không mâu thuẫn (dù không hiểu chi tiết kỹ thuật)
- [ ] Biết `track: A | B | cross` + `channel: internal | marketplace | acn` phân biệt FEAT

### ❓ 10.8 FAQ

- **"Tôi phải hiểu Kafka event schema không?"** → Không cần hiểu **cách hoạt động**, nhưng
  nên biết **event nào tồn tại** (đọc lướt `event-storming/ES-*.md`) để viết BR không mâu
  thuẫn (vd "khi handover-completed thì..." — biết event handover-completed có tồn tại).
- **"BR của tôi cần data từ boundary khác — được không?"** → Nêu BR ở góc nghiệp vụ (vd "biết
  trust score của renter"). Đừng chỉ định *lấy từ đâu* — đó là việc Architect. Nếu boundary
  đích chưa có capability đó → CR MAJOR.
- **"Anti-persona có phải viết PERSONA đầy đủ không?"** → Không. Anti-persona chỉ cần mô tả
  ngắn "ai + tại sao KHÔNG phục vụ" trong `_discovery/persona-pool.md §Anti-personas`.

---

# ═══════════ PHẦN C — CƠ CHẾ + TRA CỨU ═══════════

## §11. Hooks + Gate + STATE

### 11.1 Bảng hook (CARxRENTAL hiện tại — 8 file trong `scripts/hooks/`)

| Hook | Cung cấp | Trạng thái |
|---|---|---|
| `sessionstart.sh` (SessionStart) | In STATE + NON-NEGOTIABLES | ✅ Active |
| `userpromptsubmit.sh` (UserPromptSubmit) | Inject `[STATE: W0/DISCOVERED/agent=…]` mỗi turn | ✅ Active |
| `pretooluse-boundary-block.sh` | Chặn edit ngoài `owned_paths` | ✅ Active |
| `pretooluse-meta-edit-block.sh` | Chặn meta Edit/Write ngoài allowlist (ONBOARDING-*.md exempt vì ngoài framework) | ✅ Active (từ D3.5) |
| `pretooluse-aggregated-readonly.sh` | Chỉ `aggregator` được ghi `_aggregated/**` | ✅ Active |
| `pretooluse-question-budget.sh` | Chặn tool call khi agent vượt question budget (FM-QUESTION-BUDGET) | ✅ Active |
| `pretooluse-question-budget-floor.sh` | Enforce budget tối thiểu — reset về floor nếu bị set âm | ✅ Active |
| `precompact.sh` (PreCompact) | Pin STATE + 3 decisions mới nhất vào summary | ✅ Active |
| `pretooluse-readonly-inputs` | Chặn ghi `_inputs/**` | ⏳ Sẽ enable ở DOMAIN/ARCHITECT |
| `pretooluse-contract-hash-check` | Re-hash contract; drift → block `FM-CONTRACT-DRIFT` | ⏳ Sẽ enable ở SPECS |

### 11.2 Gate script

- `discovery-gate.py <D-wave>` — check exit gate của D-wave trước khi `state.py transition`
- `wave-gate-*.py` (sẽ có ở SPECS) — check exit của wave stage
- `lint-frontmatter.py` — chặn `/sync-to-specs` nếu artifact có jargon sai layer

### 11.3 STATE machine — 2 tầng

| Tầng | STATE.json ở đâu | Ai đọc/ghi |
|---|---|---|
| **Project-level** | `SPECS/Execution/STATE.json` (chưa có) | SM ghi, mọi trạm đọc |
| **Local service** | `<boundary>/Execution/STATE.json` (chưa có) | Dev BE/FE ghi cho service mình |

Hook `state-guard.py` chặn transition không hợp lệ.

### 11.4 Failure mode ID (thường gặp)

| Code | Nghĩa |
|---|---|
| `FM-META-EDIT` | Meta agent edit path ngoài allowlist |
| `FM-BOUNDARY-EDIT` | Agent edit ngoài `owned_paths` |
| `FM-AGGREGATED-EDIT` | Non-aggregator ghi `_aggregated/**` |
| `FM-XBOUNDARY-WRITE` | Ghi vào territory boundary khác |
| `FM-CONTRACT-DRIFT` | Contract signature không khớp |
| `FM-QUESTION-BUDGET` | Agent vượt question budget |

---

## §12. Truy vết end-to-end

Mỗi dòng code trong `-BOUNDARIES` / `-EXPERIENCES` phải lần ngược được tới H1–H6 ban đầu:

```
HYPOTHESIS (D0)  →  CAPABILITY (D1)  →  EVENT (D2)
      ↓                                        ↓
BOUNDARY (D3)  ←────────  CONTRACT (D4)  ←────┘
      ↓
CHARTER (D5)  →  WAVE (D7)
      ↓
FEATURE + BR (DOMAIN)  →  ADR + HLD (ARCHITECT)  ∥  DESIGN-SYSTEM (UIUX)
      ↓                          ↓
TEST-CASE (TESTING)  ←──  SOURCE-CODE + DECISION-REF (BOUNDARIES/EXPERIENCES)
      ↓
TEST-EXECUTION (thật)  →  BUG (nếu có) → fix → verify
      ↓
WAVE-REPORT (QA ký)  →  SHIP
```

Ví dụ trace 1 feature `"Đặt xe qua Sàn (kênh DriverPlus)"`:

```
INV-001/002 (DriverPlus→CARxRENTAL→ACN chain) + Hypothesis GMV Marketplace
  → CAP marketplace book vehicle (Phase 1 F2D)
  → ES-booking event "BookingRequested" (canonical name — ADR-CR-002..016) + "AvailabilityHoldRequested" (ADR-CR-036)
  → boundary `booking` (Go) + `marketplace` (Go) + `fleet-core` (Node — AllotmentPool SoT)
  → external touch: DriverPlus API surface qua `bff-marketplace` (channel=acn per ADR-CR-041 ACL adapter)
  → contract api/booking-query-v1.md + api/fleet-core-availability-allotment-v1.md + event/booking-payment-requested-v1.md (SHA256 ...)
  → CHARTER booking §3 capability #4 + CHARTER fleet-core AllotmentPool
  → W3 (Walking Skeleton escrow E2E) + W6 (SEAM dual-path no-oversell)
  → FEAT-Booking-Create-Marketplace + BR-Booking-Payment-Gate + BR-Availability-Hold-TTL
  → ADR-CR-036 (POST /availability/hold atomic idempotent) + ADR-CR-041 (ACL adapter DriverPlus)
  → tokens.json — booking-status-badge
  → TC-Booking-E2E-Escrow-001..015 (test W3 golden-path) + TC-AllotmentPool-Race-* (test W6)
  → src/booking/create_booking.go (# DECISION-REF: ADR-CR-036 | ADR-CR-041 | BR-Booking-Payment)
  → curl test PASS + k6 p95 <200ms + hold race no-oversell invariant xanh
  → APPROVED (W6 wave-report)
  → SHIP
```

Đây là cốt lõi "hướng sản phẩm": mọi dòng code có lý do kinh doanh truy được.

---

## §13. Người duyệt + Cấp CR

### 13.1 Năm người duyệt (recap)

| Vai trò | Người | Gác ở đâu |
|---|---|---|
| Kiến trúc | Nguyen Ha Anh `<nguhanh@cardoctor.vn>` | D3 stack, HLD/ADR, `CONTRACT_READY_GATE` |
| Delivery | Nguyen Ha Anh (kiêm) | WAVE-SEQUENCE (D7 — ADR-D7-001), `/wave-start`, `/sign-qc` |
| Business | Nguyen Ha Anh (kiêm) | D0/D1 + INV-001..005 (ADR-CR-040), `/wave-domain-ready` |
| **QA** | **`qa@cardoctor.vn`** (≠ Business) | `/wave-signoff` |
| Bảo mật | Nguyen Ha Anh (kiêm) | D3 security, review trước DEV |

### 13.2 Cấp CR — ai duyệt mức nào

| Mức độ | Ví dụ ở CARxRENTAL | Người duyệt | Lan toả (tier) |
|---|---|---|---|
| **COSMETIC** | Sửa typo trong FEAT/CHARTER | Sửa trực tiếp | Không |
| **MINOR** | Thêm field optional trong AC, làm rõ BR | Tác giả / duyệt liên quan | T2–T5 |
| **MODERATE** | Đổi behavior, contract thêm field, ADR mới | Kiến trúc (contract) / Nghiệp vụ (scope) | T1 → T2, ký lại contract |
| **MAJOR** | Đổi scope (thêm/bớt persona, capability), event schema breaking, đổi stack 1 boundary | **Nghiệp vụ + Kiến trúc** | T0 inventory, tạo lại wave/MANIFEST |
| **CRITICAL** | Đổi Dual-Track A/B, phá INV-001..005 (chuỗi DriverPlus/CARxRENTAL/ACN), đại tu stack (vd bỏ polyglot), đổi multi-tenant model | **Tất cả 5 người duyệt** (đủ quorum) | Kiểm lại toàn dự án |

### 13.3 CARxRENTAL-specific

- **Nguyen Ha Anh giữ 4/5 vai** → có thể approve **in-session** cho MODERATE trở xuống
- **QA (`qa@cardoctor.vn`) BẮT BUỘC ≠ Business** — Nguyen Ha Anh không được bypass QA
- **58 ADR/CR đã raise trong DISCOVERY** — precedent library đọc `Execution/tracking/decisions.md`
  để hiểu pattern. CR chính:
  - **ADR-D3-001..004** (baseline stack); **ADR-D7-001** (7-wave cadence)
  - **ADR-CR-020** (Keycloak override), **ADR-CR-024** (BFF NestJS+@nestjs/apollo),
    **ADR-CR-025** (Tailwind+shadcn/ui unified), **ADR-CR-026** (flutter_bloc),
    **ADR-CR-027 + CR-030** (Elastic observability + trim Metricbeat)
  - **ADR-CR-036** (AllotmentPool SoT + POST /availability/hold atomic — MUST-BY-ARCHITECTURE)
  - **ADR-CR-040** (chuỗi 3 bên DriverPlus/CARxRENTAL/ACN — INV-001..005)
  - **ADR-CR-043** (scope 3 experience nội bộ — Consumer đi qua DriverPlus)
  - **ADR-CR-046** (contract acn-consumer-trust align với ACN)
  - **ADR-CR-048** (D5 deep-review fix 39 findings)
  - **ADR-CR-049** (D7 MVP coverage — 6 Phase-1 cap bổ sung; WON'T-list còn 4)
  - Trước khi raise CR mới, grep decisions.md xem đã có precedent chưa

---

## §14. Lộ trình học CARxRENTAL (2–3 ngày)

### Ngày 1 (~6h) — Bức tranh chung
- **Sáng (3h):** đọc §0–§5 tài liệu này
- **Chiều (3h):**
  - `_aggregated/PRD.md` (30 phút)
  - `plan/WAVE-SEQUENCE.md` (45 phút)
  - 1 `CHARTER.md` mẫu của boundary/experience bạn sẽ giao (30 phút)
  - `Execution/tracking/decisions.md` — lướt 10 ADR/CR gần nhất (30 phút)
  - Thắc mắc — hỏi mentor / SM (thời gian còn lại)

### Ngày 2 (~6h) — Cẩm nang role của bạn
- **Sáng (3h):** section role bạn ở PHẦN B (§6 nếu Dev BE, §7 nếu Dev FE, §8 nếu QC, §9 nếu SM, §10 nếu BA)
- **Chiều (3h):**
  - 1 contract mẫu (loại phù hợp role — Dev BE đọc api+event, Dev FE đọc api+ui, QC đọc mọi loại, SM đọc CONTRACT-MAP, BA đọc BR)
  - 1 `ES-<domain>.md` (event storming domain relevant — 12 domain có sẵn)
  - Nếu Dev BE: đọc `_shared/coding-standards/backend-golang.md` (nếu Go) hoặc `backend-nodejs.md` (nếu Node) + `security-policy.md`
  - Nếu Dev FE: đọc `_shared/coding-standards/frontend-web.md` hoặc `frontend-mobile.md`
  - Nếu QC: đọc bug report mẫu (nếu có) hoặc plan test-cases
  - Nếu SM: bật orchestrator dashboard hands-on
  - Nếu BA: viết thử 1 BR draft

### Ngày 3 (~4–6h) — Hands-on + shadowing
- Shadow senior cùng role: theo 1 task từ đầu đến cuối
- Tick checklist role (§6.8 / §7.8 / §8.10 / §9.6 / §10.7)
- Nhận task nhỏ có mentor kèm cặp

### 14.1 Bảng tra "muốn hiểu X → đọc file nào"

| Muốn hiểu | Đọc file nào |
|---|---|
| Vì sao có dự án này + 5 Invariant | `_discovery/hypothesis-log.md` (§1 Vision + §2 Problem + §2.5 INV-001..005) |
| Phục vụ ai + không phục vụ ai + external actor | `_discovery/persona-pool.md` |
| Sản phẩm cần làm được gì (74 CAP) | `_discovery/capability-map.md` |
| Nghiệp vụ chảy thế nào | `_discovery/event-storming/ES-<domain>.md` (12 domain) |
| Hệ thống chia mảnh nào | `BOUNDARY-MAP.md` + `SYSTEM-TOPOLOGY.md` |
| Stack thật đang dùng | Bảng §4.4 file này + `_aggregated/TECHSTACK.md` |
| 1 mảnh sống vì cái gì | `<boundary|experience>/CHARTER.md` (18 CHARTER ACTIVE) |
| Đầu nối giữa 2 mảnh | `contracts/{api,event,ui,data}/*.md` (60 contract) + `CONTRACT-MAP.md` |
| Wave nào làm cái gì (7 wave) | `plan/WAVE-SEQUENCE.md` |
| Quyết định vì sao chọn X (58 ADR/CR) | `Execution/tracking/decisions.md` |
| Luật cấm / hàng rào | `CLAUDE.md §1 NON-NEGOTIABLES` + `.claude/settings.json` + `scripts/hooks/` |
| Trạng thái hiện tại | `python3 scripts/state.py summary` |
| Chuỗi phân phối DriverPlus/CARxRENTAL/ACN | `_discovery/hypothesis-log.md §2.5-2.6` + ADR-CR-040..046 trong `decisions.md` |

---

## §15. FAQ chung CARxRENTAL

### 15.1 Stack

- **"Vì sao polyglot Go + NestJS chứ không uniform 1 stack?"**
  → **ADR-D3-001**: hot-path (payment/booking/marketplace/notification/handover) cần latency
  thấp + throughput cao → Go concurrency + **Echo v4** (ADR-CR-023) minimal overhead + **GORM**
  (ADR-CR-018). CRUD-heavy (fleet-*, driver, identity, reputation, platform-ops) cần dev
  velocity + ecosystem TypeORM/NestJS. Chấp nhận chi phí learning curve 2 stack, bù lại phù
  hợp workload từng boundary.

- **"Vì sao Elastic thay vì Loki+Tempo+Prom+Grafana?"**
  → **ADR-CR-027** + **ADR-CR-030**: team có ops expertise sẵn với Elastic; 1 stack observability
  toàn diện (log + metric + trace + APM) qua **OTel Collector → Elastic OTLP native**; tránh
  maintain 4 tool riêng. Chi phí self-host EKS chấp nhận được.

- **"Vì sao Keycloak thay Cognito?"**
  → **ADR-CR-020**: cần multi-tenant realm control chi tiết; roadmap SSO tương lai; tránh
  vendor-lock AWS Cognito; self-host EKS đã có nên chi phí ops không tăng nhiều.

- **"Vì sao BFF GraphQL? Vì sao NestJS wrap Apollo?"**
  → **ADR-D3-004** chốt Apollo GraphQL BFF per persona-cluster; **ADR-CR-024** chọn **NestJS
  + `@nestjs/apollo`** thay Apollo standalone: NestJS DI + guard/interceptor phù hợp BFF
  cross-cutting (auth, rate-limit, cache); giữ đủ Apollo feature (DataLoader, federation);
  đồng bộ paradigm với 7 Node boundary cũng dùng NestJS.

- **"Vì sao Flutter mobile thay vì native / React Native?"**
  → 1 codebase iOS + Android; team đã có kinh nghiệm Flutter; performance đủ cho driver-app;
  tránh maintain 2 codebase Swift+Kotlin. State management: **`flutter_bloc` (Bloc + Cubit)**
  — **ADR-CR-026**: testable, tách UI/business logic rõ.

### 15.2 Wave sequence

- **"Track A (Fleet) và Track B (Sàn) chạy song song hay tuần tự?"**
  → **Song song từ W2/W3**. W1 Foundation là root DAG chung. W2 slice Track A (milestone bảo
  hiểm — Fleet ship standalone). W3 slice Track B (Walking Skeleton escrow E2E — de-risk sớm).
  Seam duy nhất = AllotmentPool SoT (ADR-CR-036).

- **"Vì sao Walking Skeleton W3 là de-risk quyết định?"**
  → Escrow saga (PaymentRequested→Settled→Confirmed→handover→FinalSettlement→payout→Closed)
  là **rủi ro tồn vong lớn nhất** của Track B. W3 đóng golden-path E2E sớm → nếu không close
  được → escalate CRITICAL + cân nhắc pivot scope thay vì phát hiện muộn ở W7.

- **"Vì sao chỉ 7 tuần? Đủ ship MVP không?"**
  → **KHÔNG đủ MVP đầy đủ** (đã cảnh báo trong ADR-D7-001). 7 tuần với team 10 = **pilot slice**
  (2 walking skeleton + bảo hiểm Fleet SaaS Track A). Production-ready cả 2 track ước tính
  11-12 tuần. WON'T-list (ADR-CR-049) cắt 4 Phase-2 cap để khả thi.

### 15.3 Contract + ADLC

- **"Contract 60 file — có review-able không?"**
  → Có. Chia 4 loại + `CONTRACT-MAP.md` liệt kê ownership + producer/consumer. **KHÔNG** đọc
  hết — chỉ đọc contract của boundary/experience bạn giữ + upstream/downstream trực tiếp
  (thường 3–8 contract mỗi service).

- **"Repo `-BOUNDARIES` / `-EXPERIENCES` đâu?"**
  → **CHƯA tạo.** DISCOVERY đã DISCOVERED nhưng execution repo (ARCHITECT / UIUX / TESTING /
  ORCHESTRATOR + 2 code repo) chưa bootstrap. SPECS + DOMAIN đã có (sync v1.0 xong 2026-07-13).
  Trong lúc chờ, đọc CHARTER + contract để sẵn sàng.

- **"Contract đã ký muốn đổi thì sao?"**
  → Mở **CR** theo mức nghiêm trọng (§13). Thêm field optional = MINOR; đổi behavior /
  breaking event schema = MAJOR + Architecture ký. Bump version + subscriber ký lại + hash
  mới. **KHÔNG sửa lén** — `contract-sign.py verify` sẽ tố cáo (hook `contract-hash-check`
  sẽ enable sau ở SPECS).

- **"Consumer đâu? Muốn build UI cho Consumer thuê xe làm ở đâu?"**
  → **CARxRENTAL KHÔNG có UI cho Consumer.** Consumer touch point là **DriverPlus** (external
  app, INV-001). CARxRENTAL nhận đơn qua contract `driverplus-rental-request-*` (channel=acn).
  Đề xuất build app/web cho Consumer = phá INV-001 = CR **CRITICAL**.

- **"Tôi mới join — bao giờ nhận task thật?"**
  → Đi theo checklist "người mới cần nắm" ở PHẦN B section role bạn. Tick đủ = xin task
  nhỏ có mentor kèm. 1–2 task thành công → task độc lập. Việc rủi ro cao (đụng contract
  đã ký, cross-boundary, cần `--force`) → luôn hỏi mentor + người duyệt trước.

- **"Question budget của agent là gì?"**
  → Mỗi role agent có ngân sách câu hỏi (thường 3–5) trong 1 session để tránh loop hỏi vô
  hạn. Xem `[STATE: … question_budget: N/M]`. Hết budget → agent phải quyết định, không hỏi
  thêm; nếu bí thực sự → blocker.

### 15.4 Quy trình

- **"Nếu tôi phát hiện lỗi ở DISCOVERY sau khi vào DISCOVERED thì sao?"**
  → DISCOVERY đông cứng ONE-TIME. Phát hiện lỗi → mở CR (MODERATE/MAJOR tuỳ mức) → người
  duyệt tương ứng ký → hotfix trong DISCOVERY + re-sync sang SPECS + DOMAIN. **KHÔNG** re-run
  D0–D7 từ đầu.

- **"Meta agent bảo tôi không được sửa `<file>` — tôi biết chắc phải sửa, làm sao?"**
  → 2 lựa chọn: (1) `/scope-extend <file> <symbol> "<reason>"` — approve non-additive edit
  1 lần; (2) `/cr-raise MODERATE "<title>"` — xin đổi vĩnh viễn owned_paths. Đừng bypass hook.

- **"Orchestrator dashboard hiện chưa chạy được — bí quyết?"**
  → Cần set `ADLC_ADLC_ROOT` env var trỏ đúng root chứa các repo CARxRENTAL. Chạy:
  ```bash
  ADLC_ADLC_ROOT=/home/engineer_ac/Workspaces/haips/carxrental \
    uvicorn app.main:app --port 8765
  ```
  Nếu chỉ có DISCOVERY repo → dashboard sẽ báo 6 repo còn lại "chưa tồn tại" — đúng, không phải bug.

---

> 🔚 **Hết.** Người mới: đọc **PHẦN A + section role của bạn ở PHẦN B** là đủ để bắt tay làm.
> **PHẦN C** khi cần đào sâu cơ chế kiểm soát. Học tới đâu, tick **checklist "người mới cần
> nắm"** trong section role bạn tới đó. Bí → §14.1 bảng tra + §13 hỏi người duyệt.
>
> Có góp ý / thắc mắc: raise `/cr-raise MINOR "onboarding-v1 feedback: <text>"` hoặc ping
> Architecture Authority (Nguyen Ha Anh).
