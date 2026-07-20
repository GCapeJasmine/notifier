# ADLC — Tài liệu Onboarding v4.5 (hiểu toàn bộ pipeline trong 1.5–2 ngày)

> **Mục đích:** giúp bạn nắm **100%** cách hệ thống poly-repo này hoạt động — *trước hết là câu chuyện
> sản phẩm* (xây cái gì, cho ai, theo trình tự nào, ai quyết), *sau đó* mới tới cơ chế kỹ thuật.
>
> Tổng hợp từ việc đọc cả 7 repo + orchestrator (tháng 6/2026).
> Vị trí: `nambp/ADLC-ONBOARDING-v4.5.md` (gốc poly-repo, cấp cha của 7 sibling repo).

---

## 🧭 BẢN ĐỒ TÀI LIỆU — đọc theo thứ tự này

Tài liệu chia **2 phần**. Người mới đọc **PHẦN A trước** (câu chuyện sản phẩm); PHẦN B là cẩm nang
tra cứu cơ chế máy móc, đọc khi cần đào sâu.

| | Mục | Trả lời câu hỏi |
|---|---|---|
| **PHẦN A — SẢN PHẨM** | §0 ADLC đẻ ra cái gì | "Cái dây chuyền này sản xuất ra sản phẩm gì, theo mô hình nào?" |
| *(đọc trước)* | §1 Dàn nhân vật | "**Ai** tham gia, **vai trò** gì, ai có quyền ký?" |
| | §2 DISCOVERY (D0→D7) | "Trước khi code, ta chốt những gì? **Từng sub-wave** làm gì, ai chủ trì?" |
| | §3 EXECUTION theo Wave | "**Sau Discovery**, một Wave đi qua những trạm nào, mỗi trạm ai làm gì?" |
| | §4 Trace 1 feature | "Một tính năng cụ thể chạy xuyên toàn bộ ra sao?" |
| | §5 Vì sao tạo giá trị | "Tại sao phải cầu kỳ thế này?" |
| **PHẦN B — KỸ THUẬT** | §6 Trạng thái + hàng rào kiểm soát | "Hook/gate chặn cái gì, khi nào?" |
| | §7 Lộ trình học + tra cứu | "Đọc theo lộ trình nào, bí thì tra ở đâu?" |
| | §8 Người duyệt + mức CR | "Ai ký mức thay đổi nào?" |

---

# ═══════════ PHẦN A — CÂU CHUYỆN SẢN PHẨM ═══════════

## 0. ADLC LÀ GÌ — VÀ NÓ ĐẺ RA SẢN PHẨM NHƯ THẾ NÀO

**7 repo này KHÔNG phải là sản phẩm.** Chúng là một **dây chuyền sản xuất phần mềm điều khiển bằng
AI agent**, tên **ADLC (Agentic Development Life Cycle) v4.5.0**. Việc của dây chuyền là *đẻ ra*
spec → design → code cho sản phẩm thật: **DriverPlus** (nền tảng kết nối dịch vụ ô tô).

> 📌 **"4.5" là phiên bản của DÂY CHUYỀN, không phải tên sản phẩm.** Có 2 loại "phiên bản":
> (1) *phiên bản dây chuyền* — **ADLC v4.5.0**, đổi khi ta nâng cấp chính quy trình (thêm trạm, sửa
> state machine, đổi hook, đổi contract sync); (2) *phiên bản sản phẩm* — các bản release **DriverPlus**
> mà dây chuyền 4.5 sản xuất. "Phát triển theo ADLC 4.5" = *dùng phiên bản dây chuyền 4.5 này để sản
> xuất phần mềm*.

### 0.1 Bảy repo trong nhà máy — liệt kê rõ

Nhà máy gồm **7 repo hiện có** (6 trạm vận hành + 1 bảng điều phối chỉ đọc) và **2 repo code chưa tồn tại**:

| Repo | Là trạm gì | Tạo / giữ gì (tài liệu chính) |
|---|---|---|
| `DRIVERPLUS-ADLC-DISCOVERY` | **Khảo sát** — chạy 1 lần, đặt "hiến pháp" (D0→D7) | giả thuyết sản phẩm, chân dung người dùng, năng lực nghiệp vụ, event-storming, `BOUNDARY-MAP`, bộ ADR, `contracts/*`, `CHARTER.md`, `WAVE-SEQUENCE.md` |
| `DRIVERPLUS-ADLC-DOMAIN` | **Nghiệp vụ** (PO/BA) — ngôn ngữ kinh doanh thuần | epic, tính năng + tiêu chí nghiệm thu, quy tắc nghiệp vụ, persona, wireframe → `product/*` |
| `DRIVERPLUS-ADLC-ARCHITECT` | **Kiến trúc** (kiến trúc sư giải pháp) — giải pháp kỹ thuật | thiết kế tổng thể (HLD), mô hình domain, workflow, ADR → `design/*` |
| `DRIVERPLUS-ADLC-UIUX` | **Hệ thống thiết kế** — token, component và màn hình | `DESIGN-SYSTEM.md`, `tokens.json`, `components/*`, `screens/*` → `design-systems/*` |
| `DRIVERPLUS-ADLC-SPECS` | **Băng chuyền trung tâm (HUB)** + máy trạng thái của Wave | gom mọi trạm; giữ `contracts/*` + trạng thái; `/sync-design` phát thiết kế xuống repo code |
| `DRIVERPLUS-ADLC-TESTING` | **QC** — kiểm thử thật + ký duyệt trước khi ship | kịch bản kiểm thử tích luỹ, chạy test thật, bug, WAVE-REPORT → `qc/*` |
| `DRIVERPLUS-ADLC-ORCHESTRATOR` | **Bảng điều phối chỉ đọc** (FastAPI + watchdog + React) | đọc STATE của cả 8 trạm → bảng theo dõi thời gian thực; **KHÔNG ghi state** |

**2 repo code thật — DriverPlus CHƯA tạo** (dự án đang ở đầu nguồn). Cấu trúc của chúng xem ở §3.7:

| Repo | Là gì (repo "vỏ") | Mỗi service con gồm |
|---|---|---|
| `-BOUNDARIES` | Code **backend** thật — chứa **N boundary repo con**, mỗi boundary 1 git repo riêng | `design/` (bản chụp chỉ đọc từ SPECS) + `src/`+`tests/` + máy trạng thái LOCAL + 6 agent (dev/fix/review/test-unit/contract/integration) |
| `-EXPERIENCES` | Code **frontend** thật (web/mobile) — chứa **N experience repo con** | cùng khuôn; 6 agent (dev/fix/review/test-unit/component/contract); tiêu thụ design-system (tokens) |

> 6 trạm phase (DISCOVERY → DOMAIN → ARCHITECT → UIUX → SPECS → TESTING) + 2 repo code (chưa có) hợp thành
> **8 trạm** mà ORCHESTRATOR theo dõi (xem §0.3). Bản thân ORCHESTRATOR **không** phải một trạm — nó chỉ *quan sát*.

### 0.2 Hai trục thời gian: "quy hoạch một lần" → "sản xuất lặp lại"

Sản phẩm **không** được code thẳng tay. Nó ra đời theo 2 pha tách bạch:

```
┌─────────────────────────────┐   ┌──────────────────────────────────────────────┐
│  PHA 1 — DISCOVERY           │   │  PHA 2 — EXECUTION (lặp theo từng Wave)        │
│  "Hiến pháp sản phẩm"        │   │  "Sản xuất từng lát release"                   │
│  CHẠY ĐÚNG 1 LẦN, đông cứng  │──▶│  W1 → W2 → W3 → …  (mỗi wave đi trọn 1 vòng)   │
│  D0 → D7                     │   │  nghiệp vụ→thiết kế→contract→code→QC→ship      │
└─────────────────────────────┘   └──────────────────────────────────────────────┘
        (quy hoạch nhà máy)                  (chạy băng chuyền theo đơn hàng)
```

- **PHA 1 — DISCOVERY (1 lần):** chốt *toàn bộ bản đồ nhà máy* — có những **boundary** (mảnh backend)
  nào, **experience** (mảnh frontend) nào, dùng **stack** gì, **contract** giữa các đội ra sao, chia
  làm mấy **Wave**. Đây là khâu **quy hoạch + hiến pháp**, không lặp lại (§2).
- **PHA 2 — EXECUTION (lặp):** mỗi nhóm tính năng gom vào một **Wave** (W1, W2…) theo
  `plan/WAVE-SEQUENCE.md` mà DISCOVERY đã vạch. Một Wave đi **trọn vòng**: nghiệp vụ → kiến trúc +
  thiết kế → chốt hợp đồng → code song song → QC → ship. Một bản release = tập các Wave đã được ký duyệt (§3).

### 0.3 Tám trạm — và "đơn vị sản phẩm" là Wave

```
   PHA 1                      PHA 2 (lặp cho mỗi Wave)
 DISCOVERY → DOMAIN → ARCHITECT → UIUX → [SPECS = hub] → BOUNDARIES → EXPERIENCES → TESTING
   (có)       (có)      (có)      (có)     (có)           (chưa có)     (chưa có)     (có)
  hiến pháp  nghiệp vụ  kỹ thuật  design   băng chuyền    backend       frontend     QC/ship
                                            trung tâm
```

Mỗi repo là **một trạm trong nhà máy**: nhận nguyên liệu đầu vào → xử lý bằng AI agent có kỷ luật →
đẩy bán thành phẩm sang trạm kế. **SPECS là băng chuyền trung tâm** mọi trạm khác đổ vào. 2 repo
`-BOUNDARIES`/`-EXPERIENCES` (code thật) **chưa tồn tại** — dự án đang ở đầu nguồn (DISCOVERY).

> 💡 **Đòn bẩy hiểu nhanh nhất:** đơn vị sản phẩm là **Wave**. Mọi câu hỏi "phát triển theo 4.5 cần
> làm gì" thực chất là "**một Wave đi qua những trạm nào, mỗi trạm ai làm gì**" — §3 trả lời đúng câu đó.

---

## 1. DÀN NHÂN VẬT — AI THAM GIA, VAI TRÒ GÌ

Dây chuyền có 3 *loại* tác nhân: **con người** (quyết & ký), **AI agent** (làm phần nặng), **máy móc**
(hook/gate canh luật tự động). Nắm dàn nhân vật này là đọc được mọi trạm.

### 1.1 Con người

| Nhân vật | Vai trò |
|---|---|
| **Người khởi xướng** (PO / founder) | Đưa brief/ý tưởng, định ưu tiên, mở wave |
| **5 người duyệt chính** (xem bảng dưới) | **Ký duyệt ở các cổng quan trọng** — con người giữ quyền ở đúng chỗ rủi ro cao |

**5 người duyệt chính — ai gác cổng nào:**

| Vai trò duyệt | Người | Gác / ký ở đâu |
|---|---|---|
| **Kiến trúc** | thangnvc | Chốt **stack** (D3) · duyệt HLD/ADR (ARCHITECT) · gác `CONTRACT_READY_GATE` |
| **Nghiệp vụ** | dangbh | Duyệt giả thuyết/persona/năng lực (D0–D1) · chốt đúng/sai **nghiệp vụ** (`/wave-domain-ready`) |
| **Giao hàng** | nambp.os | Duyệt tính khả thi **nhịp giao** (WAVE-SEQUENCE D7) · mở wave (`/wave-start`) · đóng wave (`/sign-qc`) |
| **QA** | datnt *(bắt buộc ≠ Nghiệp vụ)* | Cầm quyền **ký duyệt ship** (`/wave-signoff`: APPROVED/CONDITIONAL/REJECTED) |
| **Bảo mật** | ducpdd | Soi quyết định bảo mật (D3 stack, ARCHITECT) · review trước DEV |

> 🔑 Vì sao tách **QA ≠ Business**: tránh "vừa đá bóng vừa thổi còi" — người định nghĩa *đúng là gì*
> (Business) không được tự phán *đã đạt chưa* (QA).

### 1.2 AI agent — bộ 3 vai lặp lại ở mọi trạm

| Vai | Loại | Trách nhiệm |
|---|---|---|
| **meta** | 🤖 | **Nhạc trưởng** mỗi repo — điều phối, spawn agent, chạy transition/sync/gate. **KHÔNG tự viết artifact.** |
| **`<x>`-author** | 🤖 | **Thợ viết** artifact chuyên môn (po / ba / sa / adr / ds / qa / charter-author / capability-mapper / event-stormer / aggregator…). |
| **`<x>`-translator** | 🤖 | **Thợ dịch** artifact "ngôn ngữ phase" → format chuẩn SPECS (thêm/bớt field, gắn frontmatter audit). |

Vài author/agent chuyên biệt đáng nhớ:

| Agent | Việc đặc thù |
|---|---|
| **contract-steward** | Ký & canh **hợp đồng** (contract) — niêm phong bằng hash, chống sửa ngầm |
| **qa-executor** | Chạy test **THẬT** (curl / Playwright / k6 / axe), tạo bug — không "giả vờ PASS" |
| **aggregator** | Role *duy nhất* được ghi `_aggregated/**` — render PRD/ROADMAP, không thêm thông tin mới |
| **standards-enricher** | Điền giá trị stack thật vào chuẩn dùng chung (`_shared/*`) |

> 🧠 **Ghi nhớ bộ 3:** **meta điều phối → author viết → translator dịch sang SPECS.** Nắm 3 vai này
> là đọc được mọi trạm — phần còn lại chỉ khác *nội dung*, không khác *vai*.

### 1.3 Máy móc — trọng tài tự động

| Tác nhân | Loại | Trách nhiệm |
|---|---|---|
| **hook** | ⚙️ | Hàng rào enforcement: chặn edit ngoài `owned_paths`, chặn ghi `_inputs`, re-hash contract, inject `[STATE: …]` mỗi turn |
| **gate script** (`*-gate.py`) | ⚙️ | Trọng tài exit gate: chưa đủ điều kiện rời stage → **block** (chỉ qua bằng `--force --reason`, ghi audit) |

→ AI **không thể "đi tắt"** kể cả khi muốn; luật do **máy tự áp**, không dựa vào agent tự giác (chi tiết §6).

---

## 2. DISCOVERY — "HIẾN PHÁP SẢN PHẨM" (chạy 1 lần, D0→D7)

> ⚠️ **Đây là phần đáng đầu tư kỹ nhất.** DISCOVERY chạy **đúng 1 lần** rồi **đông cứng** mọi quyết định
> nền tảng — boundary nào, stack gì, contract nào, chia mấy wave. **Sai ở đây = sai cả nhà máy**; mọi trạm
> sau (DOMAIN/ARCHITECT/UIUX/SPECS/TESTING) đều *đọc* kết quả đã đông cứng này làm đầu vào. Nguồn gốc đầy đủ:
> `DISCOVERY/DISCOVERY-RESPONSIBILITIES.md`.

### 2.1 Sợi chỉ xuyên suốt — vì sao bước này quyết định cả sản phẩm

Discovery không phải "viết tài liệu cho có". Nó nối **một sợi chỉ** từ *điều ta tin về kinh doanh* xuống
tới *thứ tự code* — mỗi nấc khoá nấc kế, không nấc nào tự ý:

```
ta tin điều gì (D0) ─▶ ai dùng + cần làm được gì + để đạt kết quả nào (D1) ─▶ nghiệp vụ chảy ra sao (D2)
   ─▶ cắt hệ thống thành mảnh + chọn công nghệ (D3) ─▶ niêm phong hợp đồng giữa các đội (D4) ─▶ hiến chương mỗi mảnh (D5)
   ─▶ gộp thành 4 tài liệu tổng (D6) ─▶ xếp thứ tự các đợt làm (D7)
```

**Đọc ví dụ cho dễ:** ở D0 ta đặt một mục tiêu kinh doanh (vd "X% người dùng tạo hồ sơ xe trong 90 ngày")
→ D1 buộc phải có một "năng lực" tương ứng để đạt mục tiêu đó → D3 phải có một "mảnh" hệ thống giữ dữ liệu
hồ sơ xe → D4 sinh "hợp đồng" cho mảnh đó → D7 **bắt buộc** xếp mảnh đó vào đợt làm sớm. Tức là **một mục
tiêu kinh doanh ở đầu sẽ kéo theo cả quyết định kỹ thuật lẫn thứ tự làm việc** — nghiệp vụ dẫn dắt kỹ
thuật, không phải ngược lại.

**4 thói quen khiến Discovery thực sự bám sát sản phẩm (chứ không chỉ là thủ tục giấy tờ):**

1. **Khai cả "điều KHÔNG làm" và "người KHÔNG phục vụ"** — ở D0/D1 ta không chỉ ghi *ta tin gì / phục vụ
   ai*, mà còn ghi rõ **điều ta tin là KHÔNG đúng** (anti-hypothesis) và **người ta cố tình KHÔNG phục vụ**
   (anti-persona — vd kẻ spam, đối thủ trá hình). Đây là cách chặn việc ôm đồm lan man.
2. **Một bộ nguyên tắc bất di bất dịch (invariant)** — vd "dữ liệu là của người dùng", "lịch sử không sửa
   được". Mọi quyết định sau phải tôn trọng các nguyên tắc này → không có quyết định "tuỳ hứng".
3. **Mỗi năng lực gắn với một kết quả kinh doanh + mức ưu tiên** — biết năng lực này phục vụ mục tiêu nào
   và quan trọng tới đâu (cao/vừa/thấp). Cái ưu tiên cao sẽ phải làm ở đợt đầu.
4. **Sửa thì sửa NGAY trong Discovery, không "quay xe" giữa lúc đang code** — phát hiện cần thêm người dùng
   hay thêm mảnh thì điều chỉnh ngay ở D0–D7 (người phụ trách có thể gật/bác tại cổng). Sau khi đã đông cứng,
   muốn đổi nền tảng phải mở "yêu cầu thay đổi" (CR) có người ký — không sửa lén.

### 2.2 Bản đồ nhanh 10 sub-wave

| Sub-wave | Câu hỏi sản phẩm cốt lõi | Người/agent chủ trì | Đẻ ra (chốt cái gì) |
|---|---|---|---|
| **D0** | Ta tin điều gì, đo bằng cách nào? | 👤 **Business** (thủ công) | `hypothesis-log.md` — giả thuyết + **anti-hypothesis** |
| **D1** | Ai dùng, cần năng lực gì, để đạt KPI nào? | 🤖 capability-mapper (Business + Architecture **đồng tác giả**) | `persona-pool.md` (+anti-persona), `capability-map.md` (+business outcome + candidate domain) |
| **D2** | Nghiệp vụ chảy qua sự kiện nào? | 🤖 event-stormer (1 spawn/domain, song song) | `event-storming/ES-<domain>.md` (≥10 event + aggregate + hot-spot) |
| **D3** | Tách thành boundary/experience nào? Stack gì? | 👤 **Architecture** + 🤖 charter-author | `BOUNDARY-MAP`, `SYSTEM-TOPOLOGY`, skeleton + **ADR chốt stack** |
| **D3.5** | Biến "stack đã chốt" thành chuẩn đọc-được | 🤖 standards-enricher | `_shared/*` đã fill + `ENRICHMENT-LOG.md` |
| **D4** | Niêm phong giao kèo giữa các đội | 🤖 contract-steward (1 spawn/loại) | `contracts/{api,ui,data,event}/*` RATIFIED + **chữ ký hash** |
| **D5** | Mỗi mảnh sống vì điều gì, sở hữu cái gì? | 🤖 charter-author (1 spawn/target) | `<target>/CHARTER.md` **ACTIVE** (§1–§9) |
| **D6** | Gộp lại thành bức tranh tổng (không thêm mới) | 🤖 aggregator (render-only) | `_aggregated/{PRD,ROADMAP,SYSTEM-ARCHITECTURE,TECHSTACK}.md` |
| **D7** | Làm theo thứ tự wave nào? | 🤖 charter-author (mode WAVE-SEQUENCE) + 👤 **Delivery** | `plan/WAVE-SEQUENCE.md` **ACTIVE** + ADR cadence |
| **DISCOVERED** | Đóng pha, niêm phong cả gói | 🤖 meta + 👤 ≥1 người duyệt ký | (đã đông cứng) → bắt buộc `/sync-to-specs` + `/sync-to-domain` |

> 🤖 = AI agent; 👤 = con người duyệt. Mọi sub-wave có **cổng rời tự động**: `state.py transition`
> gọi `discovery-gate.py <D-wave>` kiểm exit gate; **fail → block**, chỉ qua bằng `--force --reason`
> (ghi audit). **meta** điều phối xuyên suốt — không tự viết artifact.

---

> 📖 **Cách đọc 10 mục dưới (đơn giản hoá cho người mới):** mỗi mục giải thích bằng tiếng Việt, chữ
> chuyên môn để trong ngoặc lần đầu để bạn tra lại khi gặp trong file/lệnh. Mỗi mục có **✅ checklist
> "người mới cần nắm"** ở cuối — tick được hết là bạn hiểu sub-wave đó.

#### 🟦 D0 — Đặt giả thuyết · *"Ta tin điều gì? Đo đúng/sai bằng cách nào?"*
- 👥 **Ai làm:** người phụ trách nghiệp vụ tự viết tay — **chưa** gọi AI và **chưa**
  mời người kỹ thuật, để tránh sa đà vào công nghệ quá sớm.
- ⚙️ **Làm gì (nói đơn giản):** biến ý tưởng còn mơ hồ thành những **"điều ta tin" có thể đo đúng/sai**
  (gọi là *giả thuyết — hypothesis*), mỗi điều kèm cách đo. Đồng thời viết cả **"điều ta tin là KHÔNG đúng /
  KHÔNG làm"** (*anti-hypothesis*) để khỏi ôm đồm, và vài **nguyên tắc bất di bất dịch** của sản phẩm
  (*invariant* — vd "dữ liệu là của người dùng").
- 📤 **Tạo ra file:** `_discovery/hypothesis-log.md`.
- ❄️ **Chốt cứng cái gì:** sản phẩm này LÀ gì / KHÔNG là gì, và đo thành công bằng cách nào. Đây là gốc
  của **mọi** câu hỏi "vì sao làm cái này".
- 🚪 **Khi nào coi là xong:** có tầm nhìn, có mô tả vấn đề, **≥3 giả thuyết đo được**, **≥2 điều "không làm"**.
- ➡️ **Bước sau dùng để:** D1 từ giả thuyết suy ra ai dùng + cần làm được gì.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu: "giả thuyết" khác "mong muốn" ở chỗ phải **đo được** đúng/sai
  - [ ] Mở `hypothesis-log.md`, chỉ ra đâu là tầm nhìn, đâu là "điều không làm" (anti-hypothesis)
  - [ ] Trả lời được: vì sao phải viết cả phần "điều KHÔNG làm"?

#### 🟦 D1 — Vẽ bản đồ năng lực · *"Ai dùng? Cần làm được gì? Để đạt kết quả nào?"*
- 👥 **Ai làm:** một AI (*capability-mapper*) hỏi tối đa 5 câu; người kinh doanh + người kỹ thuật **cùng
  làm** (kinh doanh nói *cần gì*, kỹ thuật soi *có khả thi không*).
- ⚙️ **Làm gì (nói đơn giản):** từ giả thuyết → liệt kê **người dùng** (*persona*) và cả **người ta cố
  tình KHÔNG phục vụ** (*anti-persona* — vd kẻ spam, đối thủ trá hình); mỗi nhóm người cần những **năng
  lực** (*capability*) gì; mỗi năng lực gắn với **kết quả kinh doanh** nó mang lại + **mức ưu tiên**
  (cao/vừa/thấp); rồi gom năng lực thành các **nhóm chủ đề** (*candidate domain*) để bước sau mổ xẻ.
- 📤 **Tạo ra file:** `_discovery/persona-pool.md`, `_discovery/capability-map.md`.
- ❄️ **Chốt cứng cái gì:** ta phục vụ ai, mỗi năng lực phục vụ mục tiêu kinh doanh nào, cái nào làm trước.
- 🚪 **Khi nào coi là xong:** ≥1 người dùng + **≥2 "không phục vụ"**; **≥5 năng lực** (mỗi cái gắn người
  dùng + kết quả + nhóm chủ đề).
- ➡️ **Bước sau dùng để:** D2 mổ xẻ từng nhóm chủ đề; D7 dùng mức ưu tiên xếp việc nào làm trước.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Phân biệt "người dùng" (persona) vs "người không phục vụ" (anti-persona); nêu 1 ví dụ mỗi loại
  - [ ] Mở `capability-map.md`, chỉ 1 năng lực và nói nó phục vụ kết quả kinh doanh nào
  - [ ] Hiểu vì sao mỗi năng lực phải có mức ưu tiên (liên quan tới việc xếp đợt làm sau)

#### 🟦 D2 — Dựng dòng chảy nghiệp vụ · *"Chuyện gì xảy ra, theo trình tự nào?"*
- 👥 **Ai làm:** một AI (*event-stormer*), mỗi nhóm chủ đề một con, **chạy song song**.
- ⚙️ **Làm gì (nói đơn giản):** với mỗi nhóm, liệt kê các **"sự việc đã xảy ra"** trong nghiệp vụ (*event*
  — vd "đơn đã đặt", "xe đã giao") theo dòng thời gian; ai/hành động nào gây ra chúng; những **cụm dữ liệu
  phải nhất quán cùng nhau** (*aggregate*); và những **điểm còn mơ hồ** (*hot-spot*) để dành cho người kỹ
  thuật giải sau.
- 📤 **Tạo ra file:** `_discovery/event-storming/ES-<chủ đề>.md` (mỗi nhóm một file).
- ❄️ **Chốt cứng cái gì:** bản chất nghiệp vụ diễn ra qua những sự việc gì; cụm dữ liệu nào đi liền nhau.
- 🚪 **Khi nào coi là xong:** mỗi nhóm **≥10 sự việc** + ≥1 cụm dữ liệu + có khai báo điểm mơ hồ (kể cả
  "không có điểm nào").
- ➡️ **Bước sau dùng để:** D3 dựa "cụm dữ liệu" để cắt hệ thống thành mảnh; D4 dựa "sự việc" để soạn hợp đồng.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu "sự việc nghiệp vụ" (event) là gì qua 2–3 ví dụ thật trong file ES
  - [ ] Chỉ ra 1 "cụm dữ liệu đi liền nhau" (aggregate) và vì sao chúng đi liền
  - [ ] Tìm 1 "điểm mơ hồ" (hot-spot) và hiểu vì sao tạm hoãn cho người kỹ thuật

#### 🟨 D3 — Cắt hệ thống thành mảnh + chọn công nghệ · *"Gồm những mảnh nào? Xây bằng gì?"*
- 👥 **Ai làm:** người phụ trách kiến trúc dẫn dắt + một AI (*charter-author*).
  Đây là **cổng kỹ thuật quan trọng nhất** của Discovery (có người phụ trách bảo mật soi cùng).
- ⚙️ **Làm gì (nói đơn giản):** chia hệ thống thành các **mảnh backend** (*boundary*) và các **mặt giao
  diện người dùng** (*experience* — web/mobile); vẽ bản đồ các mảnh + sơ đồ tổng; tạo **khung thư mục +
  bản "hiến chương" nháp** (*CHARTER*) cho từng mảnh; và **chọn công nghệ** (*stack*) bằng các "quyết định
  kiến trúc" (*ADR*) — ngôn ngữ/khung backend, khung + bộ giao diện frontend, hạ tầng.
- 📤 **Tạo ra file:** `BOUNDARY-MAP.md`, `SYSTEM-TOPOLOGY.md`, khung thư mục từng mảnh, các ADR chốt công
  nghệ (trong `Execution/tracking/decisions.md`).
- ❄️ **Chốt cứng cái gì:** bản đồ các mảnh + công nghệ. Sau đây **không tự ý vẽ lại** ranh giới hay đổi
  công nghệ giữa chừng (phải qua "yêu cầu thay đổi" — CR).
- 🚪 **Khi nào coi là xong:** bản đồ đủ phần; mỗi mảnh có khung + hiến chương nháp; **đủ ADR chốt công nghệ**.
- ➡️ **Bước sau dùng để:** D3.5 lấy công nghệ điền vào chuẩn dùng chung; D4 biết cặp ai-gửi-ai-nhận; D5 viết
  hiến chương đầy đủ.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Phân biệt "mảnh backend" (boundary) vs "mặt giao diện" (experience); cho ví dụ
  - [ ] Mở `BOUNDARY-MAP.md`, đọc ra hệ thống gồm mấy mảnh
  - [ ] Tìm trong `decisions.md` 1 quyết định chốt công nghệ (ADR) và đọc lý do chọn

#### 🟨 D3.5 — Điền công nghệ vừa chốt vào chuẩn dùng chung · *"Lấp các chỗ để trống"*
- 👥 **Ai làm:** một AI (*standards-enricher*) làm cơ học — không tự nghĩ ra luật mới.
- ⚙️ **Làm gì (nói đơn giản):** trong các file chuẩn dùng chung (`_shared/*`: quy ước viết code, bảo mật,
  vận hành…) có nhiều **chỗ để trống** dạng `{{…}}`. Bước này điền **giá trị thật** từ công nghệ vừa chốt
  ở D3 vào (vd điền tên công cụ truy vấn dữ liệu, nền tảng triển khai), hoặc đánh dấu "không áp dụng". Đặt
  giữa D3 và D4 để **không để lọt chỗ trống** xuống các bước sau.
- 📤 **Tạo ra file:** `_shared/*` đã điền + `_shared/ENRICHMENT-LOG.md` (ghi lại từng chỗ đã điền).
- 🚪 **Khi nào coi là xong:** không còn chỗ `{{…}}` nào chưa điền.
- ➡️ **Bước sau dùng để:** D4/D5/D6 đọc chuẩn đã đầy đủ, không vấp phải chỗ trống.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu "chỗ để trống `{{…}}`" là gì và vì sao phải điền trước D4
  - [ ] Mở `ENRICHMENT-LOG.md`, xem 1 dòng: chỗ nào được điền giá trị gì

#### 🟧 D4 — Niêm phong hợp đồng giữa các đội · *"Cố định đầu nối TRƯỚC khi code"*
- 👥 **Ai làm:** một AI (*contract-steward*), mỗi loại hợp đồng một con (api / giao diện / dữ liệu / sự
  kiện), chạy song song. Mỗi bên "nhận" hợp đồng phải **ký**.
- ⚙️ **Làm gì (nói đơn giản):** với mỗi việc cần phối hợp giữa 2 mảnh, soạn một **"hợp đồng"** (*contract*)
  mô tả chính xác đầu nối: **gửi gì, nhận gì, lỗi ra sao**. Sau đó duyệt (*ratify*) và **ký niêm phong bằng
  mã băm** (*hash* — như đóng dấu xi, đụng vào là lộ).
- 📤 **Tạo ra file:** `contracts/{api,ui,data,event}/*.md` (đã duyệt), `CONTRACT-MAP.md`, file chữ ký từng bên.
- ❄️ **Chốt cứng cái gì:** đầu nối giữa các đội — **bất biến**. Muốn đổi phải ra phiên bản mới **+ ký lại**.
- 🚪 **Khi nào coi là xong:** mọi việc phối hợp có ≥1 hợp đồng đã duyệt; chữ ký kiểm tra hợp lệ.
- ➡️ **Bước sau dùng để:** D5 hiến chương trỏ tới hợp đồng; backend/frontend sau này **code song song** không
  lệch đầu nối.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu "hợp đồng" (contract) là gì — vì sao nó cho phép 2 đội làm song song
  - [ ] Mở 1 file trong `contracts/`, đọc đầu nối: gửi gì / nhận gì
  - [ ] Hiểu "ký bằng mã băm (hash)" để làm gì (chống sửa lén)

#### 🟩 D5 — Viết hiến chương đầy đủ cho mỗi mảnh · *"Mỗi mảnh sống vì điều gì, sở hữu cái gì?"*
- 👥 **Ai làm:** một AI (*charter-author*), mỗi mảnh một con.
- ⚙️ **Làm gì (nói đơn giản):** với mỗi mảnh, viết đầy đủ bản **"hiến chương"** (*CHARTER*, 9 mục): mảnh này
  tồn tại vì **sứ mệnh** gì, **sở hữu dữ liệu** nào, **cung cấp năng lực** gì (trỏ tới hợp đồng), **phụ thuộc**
  ai, **được phép sửa thư mục nào** (*owned paths* — sau này thành luật chặn), **yêu cầu chất lượng** (tốc
  độ, bảo mật), việc gì **ngoài phạm vi**, **ai có quyền duyệt**. Chuyển từ nháp → chính thức.
- 📤 **Tạo ra file:** `<mảnh>/CHARTER.md` (chính thức).
- ❄️ **Chốt cứng cái gì:** bản đặc tả ràng buộc của mỗi mảnh — là **đầu vào trực tiếp** cho D6 và D7.
- 🚪 **Khi nào coi là xong:** mọi hiến chương chính thức; mục năng lực chỉ trỏ hợp đồng đã duyệt; thư mục
  "được phép sửa" khớp thực tế.
- ➡️ **Bước sau dùng để:** D6 gộp thành tài liệu tổng; D7 xếp năng lực vào đợt làm.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Mở 1 `CHARTER.md`, chỉ ra: sứ mệnh, dữ liệu sở hữu, thư mục được phép sửa
  - [ ] Hiểu "thư mục được phép sửa" (owned paths) sẽ thành luật chặn edit ở các bước sau
  - [ ] Hiểu hiến chương KHÔNG phải tài liệu tham khảo, mà là **đặc tả ràng buộc**

#### 🟧 D6 — Gộp tất cả thành 4 tài liệu tổng · *"Chỉ gom lại, không thêm gì mới"*
- 👥 **Ai làm:** một AI (*aggregator*) — con **duy nhất** được ghi vào `_aggregated/**`; gặp mâu thuẫn nguồn
  thì **dừng**, không tự sửa.
- ⚙️ **Làm gì (nói đơn giản):** **không thêm thông tin mới** — chỉ gom mọi thứ D0–D5 thành 4 tài liệu tổng
  dễ đọc: **yêu cầu sản phẩm** (*PRD*), **lộ trình** (*ROADMAP*), **kiến trúc hệ thống**, **danh mục công
  nghệ** (*TECHSTACK*). Mỗi mục có dòng trỏ về nguồn. Chạy lại cho kết quả y hệt.
- 📤 **Tạo ra file:** `_aggregated/{PRD, ROADMAP, SYSTEM-ARCHITECTURE, TECHSTACK}.md`.
- 🚪 **Khi nào coi là xong:** 4 file gom xong, có trỏ nguồn rõ; chạy lại lần 2 không đổi gì.
- ➡️ **Bước sau dùng để:** D7 đọc lộ trình để xếp đợt; sau khi đông cứng, 4 file này thành **tài liệu sản
  phẩm chính thức** ở SPECS.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu D6 chỉ "gom lại", không tự đẻ thông tin mới
  - [ ] Mở `PRD.md`, xem 1 mục và dòng "Nguồn" của nó trỏ về đâu

#### 🟥 D7 — Xếp thứ tự các đợt làm · *"Làm theo trình tự nào để ghép khít?"*
- 👥 **Ai làm:** một AI (*charter-author*, chế độ xếp đợt) + người phụ trách giao hàng.
- ⚙️ **Làm gì (nói đơn giản):** chia toàn bộ việc thành các **"đợt"** (*Wave*): mỗi đợt làm gì, theo **kiểu
  cắt nào** (xem §3.1), gồm mảnh nào, tính năng nào, hợp đồng nào, đạt gì thì coi là xong. Ghi quyết định về
  **nhịp giao** (vd 2 tuần một đợt).
- 📤 **Tạo ra file:** `plan/WAVE-SEQUENCE.md`.
- ❄️ **Chốt cứng cái gì:** **lịch sản xuất** — SPECS dùng đúng thứ tự này để mở từng đợt.
- 🚪 **Khi nào coi là xong:** **mọi mảnh đều nằm trong ít nhất 1 đợt**; mỗi đợt hợp lệ; có quyết định nhịp giao.
- ➡️ **Bước sau dùng để:** là lịch mà toàn bộ PHA 2 (§3) lặp lại theo.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu "đợt làm" (Wave) là đơn vị sản xuất — mở `WAVE-SEQUENCE.md` xem đợt 1 gồm gì
  - [ ] Hiểu vì sao năng lực ưu tiên cao (từ D1) phải nằm ở đợt đầu

#### ⬛ DISCOVERED — Đóng giai đoạn & niêm phong cả gói · *"Đã đủ & đông cứng"*
- 👥 **Ai làm:** một AI (*meta*) chạy đóng + đẩy dữ liệu; **≥1 người phụ trách ký** xác nhận (kiến trúc soi
  bản đồ + công nghệ · kinh doanh soi giả thuyết + người dùng · giao hàng soi lịch đợt).
- ⚙️ **Làm gì (nói đơn giản):** **không tạo gì mới**. Chạy kiểm tra cuối, soát lại **toàn bộ D0–D7 cùng lúc**.
  Sau đó cả repo trở thành **"tài liệu tham chiếu đông cứng"**.
- 🚪 **Bắt buộc sau đó:** đẩy dữ liệu sang SPECS (`/sync-to-specs`) **+** sang DOMAIN (`/sync-to-domain`) —
  không đẩy thì SPECS không có gì để mở đợt.
- 🔒 **Quy tắc một-lần:** từ đây **không quay lại** sửa D0–D7. Phát hiện mới về sau xử lý qua "yêu cầu thay
  đổi" (CR) có người ký, **không làm lại từ đầu**.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu vì sao Discovery "đông cứng" — đổi nền tảng về sau phải qua CR có người ký
  - [ ] Biết 2 lệnh bắt buộc sau khi đóng: `/sync-to-specs` + `/sync-to-domain`

> 💎 **Tóm lại — vì sao Discovery là "hiến pháp":** sau khi đông cứng, *bản đồ các mảnh*, *công nghệ đã chốt*,
> *hợp đồng đã niêm phong*, và *thứ tự các đợt* đều **bất biến**. Mọi trạm sau chỉ *điền nội dung vào khung
> này*, không vẽ lại khung. Vì thế mọi thay đổi nền tảng đều phải đi qua cổng "yêu cầu thay đổi" (CR) có
> người ký — việc phình to phạm vi trở nên *nhìn thấy được*, không giấu được trong code.

---

## 3. EXECUTION — MỘT WAVE ĐI QUA NHỮNG TRẠM NÀO (sau DISCOVERY)

> Sau khi DISCOVERY đông cứng, dự án bước vào **PHA 2**: lặp lại một vòng sản xuất cho từng Wave. Mục này
> trả lời thẳng câu *"sau Discovery thì execution cần làm gì"* — theo đúng thứ tự thời gian một Wave.

### 3.1 Một "Wave" là gì — và 2 trục định hình nó

Một **Wave** = một lát sản phẩm đi trọn vòng *nghiệp vụ → ship*. Mỗi wave được mô tả bằng **2 trục độc lập**
(chốt ở D7, lưu trong `WAVE-SEQUENCE.md` + seed vào SPECS `STATE.json`):

| Trục | Giá trị | Ý nghĩa sản phẩm |
|---|---|---|
| **`wave_class`** (quy mô + độ sâu test) | `slice` | Lát mỏng (~1 ngày), test cấp 1 — đẩy nhanh một mảnh độc lập |
| | `integration` | Lát đầy đủ (~3–5 ngày), test toàn diện — ghép nhiều mảnh, kiểm tích hợp |
| **`wave_strategy`** (hình thái cắt) | `vertical` | Cắt **dọc** 1 EPIC: ghép FEAT backend + FEAT frontend cùng chủ đề → ra giá trị end-to-end |
| | `horizontal-be` | Chỉ **backend** (≤3 boundary), chưa có UI — dựng nền dữ liệu/logic |
| | `horizontal-fe` | Chỉ **frontend** (≤3 experience), tiêu thụ contract ACTIVE từ wave trước |

> 🔒 **Ràng buộc cứng:** ≤3 target/layer mỗi wave (ngân sách context ≤80KB/agent); wave `horizontal-*`
> phải đúng **một** tầng (BE *hoặc* FE, không trộn). → giữ mỗi wave nhỏ, ghép khít, dễ kiểm.

### 3.2 State machine của một Wave (xương sống ở SPECS)

```
PLANNING → DOMAIN → ARCHITECT → CONTRACT_READY_GATE → SYNC_DESIGN
        → PARALLEL_WORK → DEV_READY_GATE → QC → WAVE_END        (+ BLOCKED escape hatch)
  ├─────── đơn luồng ────────┤ │ ├──────────── song song ───────────┤
   (chốt nghiệp vụ + thiết kế   │   (BE + FE + test-plan chạy đồng thời,
    + ký contract trước)        │    chỉ đồng bộ qua contract đã ký)
                          điểm khoá: contract ký xong mới được rẽ song song
```

**Phát kiến cốt lõi của v4.x:** tách **đơn luồng PLANNING** (chốt + ký contract) khỏi **song song WORK**
(dev BE + dev FE + test-plan chạy độc lập). *Vì sao song song khả thi:* contract (api/ui/event/data) là
**single source of truth** — FE code dựa trên `contracts/api/*` (mock ở `services/`), **không cần biết BE
đã implement chưa**. Khoá contract trước = cởi trói song song sau.

### 3.3 — BƯỚC 2: SPECS mở Wave & lập kế hoạch
- 🎯 **Mục tiêu:** chọn wave để chạy, đưa cả hệ thống vào trạng thái "đang làm Wave N".
- 👥 **Ai:** **meta** ở SPECS gọi `/wave-start N`. 👤 **Delivery** chịu trách nhiệm nhịp giao.
- ⚙️ **Diễn ra:** `DISCOVERED → PLANNING → DOMAIN` (SPECS chuyển stage, "nhường sân" cho DOMAIN); refresh
  CHARTER + MANIFEST, chốt các quyết định còn treo với người duyệt.
- 💎 **Giá trị:** một **nguồn sự thật duy nhất** về tiến độ wave; orchestrator đọc state này hiển thị
  dashboard cho cả team — mọi trạm biết "đang ở Wave nào, stage nào" để không giẫm chân.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu SPECS là "nhạc trưởng tiến độ" — giữ trạng thái đợt làm cho cả hệ thống
  - [ ] Chạy `state.py summary` ở SPECS, đọc đang ở đợt/stage nào
  - [ ] Hiểu `/wave-start N` làm gì (chuyển hệ thống sang trạng thái "đang làm đợt N")

### 3.4 — BƯỚC 3: DOMAIN — "Feature làm gì, cho ai?" *(ngôn ngữ kinh doanh thuần)*
- 🎯 **Mục tiêu:** mô tả tính năng dưới góc nhìn **nghiệp vụ**, không lẫn kỹ thuật.
- 👥 **Ai:** **po-author** viết EPIC / FEATURE / JOURNEY; **ba-author** viết BUSINESS-RULE / PERSONA /
  WIREFRAME; **domain-translator** dịch sang draft SPECS. 👤 **Business** chốt đúng/sai nghiệp vụ.
- 📦 **Artifact sản phẩm (ai sở hữu):**

  | Artifact | Chủ | Chứa gì |
  |---|---|---|
  | **EPIC** | PO | Tầm nhìn + hypothesis + success metric của một nhóm tính năng |
  | **FEATURE** | PO | **Acceptance criteria** ngôn ngữ thường + giá trị người dùng + priority |
  | **JOURNEY** | PO | Hành trình trải nghiệm + touchpoint + cảm xúc (ngữ cảnh cho FEATURE) |
  | **BUSINESS-RULE** | BA | Điều kiện + ngoại lệ + phạm vi dữ liệu (translator thêm `enforcement_location: TBD-engineer`) |
  | **PERSONA** | BA | Hồ sơ actor + mục tiêu + nỗi đau + bối cảnh |
  | **WIREFRAME** | BA | Mockup low/mid-fi + luồng UX |

- 🚧 **Kỷ luật AC (acceptance criteria):** viết kiểu *"Khi khách hàng nhấp [nút], hệ thống kiểm tra điều
  kiện theo BR-xxx; nếu đạt hiện form, nếu không hiện lý do"* — **cấm jargon kỹ thuật** (endpoint, JWT,
  JSON, SQL, tên component, tên layer). Hook `lint-frontmatter.py` chặn sync nếu lọt jargon → giữ
  `product/*` thuần nghiệp vụ, người kinh doanh đọc hiểu không cần biết code.
- ⚙️ **Diễn ra:** `DOMAIN_INTAKE → AUTHORING → TRANSLATING → SYNCED`; `/sync-to-specs` ghi
  `SPECS/.../product/*` (**overwrite** — DOMAIN là source-of-truth nghiệp vụ). SPECS `/wave-domain-ready`
  → stage `ARCHITECT`.
- 💎 **Giá trị:** **hợp đồng nghiệp vụ** rõ ràng — kỹ sư & QA về sau biết chính xác "đạt thế nào là đúng".
- ✅ **Checklist người mới cần nắm:**
  - [ ] Phân biệt EPIC / FEATURE / BUSINESS-RULE qua 1 ví dụ mỗi loại
  - [ ] Hiểu "tiêu chí chấp nhận" (acceptance criteria) và vì sao cấm chữ kỹ thuật trong đó
  - [ ] Mở 1 FEATURE, kiểm: có lọt từ kỹ thuật (endpoint, JSON…) không?

### 3.5 — BƯỚC 4: ARCHITECT + UIUX — "Xây như thế nào?" *(chạy song song)*
- 🎯 **Mục tiêu:** quyết **giải pháp kỹ thuật** (ARCHITECT) và **hệ thiết kế giao diện** (UIUX).
- 👥 **Ai:**
  - **ARCHITECT:** **sa-author** (HLD, domain-model, workflow), **adr-author** (ADR: Context→Decision→
    Alternatives≥2→Consequences), **architect-translator**. 👤 **Architecture** + **Security** soi.
  - **UIUX (song song):** **ds-author** (`DESIGN-SYSTEM.md`, `tokens.json` → codegen CSS vars/theme,
    `components/*.md`, `screens/*`). 👤 **Người duyệt thiết kế** ký `APPROVED`.
- 🚧 **Chống drift đối xứng:** ARCHITECT **pull full SPECS** (`sync-from-specs`) để không thiết kế lệch
  nghiệp vụ; hook `lint-design.py` **cấm product jargon** (persona, acceptance criteria) — ngược chiều với
  DOMAIN, giữ mỗi tầng đúng vai.
- ⚙️ **Diễn ra:** cả hai `/sync-to-specs` → `SPECS/.../design/*` + `SPECS/design-systems/*` (overwrite).
  UIUX liên hệ experience **gián tiếp qua SPECS**: experience khai `design_system: <ds>` trong MANIFEST →
  `/sync-design` kéo xuống code repo. SPECS `/wave-architect-ready` → `CONTRACT_READY_GATE`.
- 💎 **Giá trị:** **bản thiết kế kỹ thuật + design system dùng chung**, đã rà nhất quán với nghiệp vụ.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Phân biệt việc của ARCHITECT (giải pháp kỹ thuật) vs UIUX (giao diện)
  - [ ] Hiểu "chống lệch" (anti-drift): vì sao ARCHITECT phải kéo full SPECS về trước khi thiết kế
  - [ ] Biết một quyết định kiến trúc (ADR) gồm gì: bối cảnh → quyết định → ≥2 lựa chọn → hệ quả

### 3.6 — BƯỚC 5: SPECS chốt hợp đồng & phát design — "Niêm phong giao kèo"
- 🎯 **Mục tiêu:** đóng băng **contract** (api/ui/data/event) rồi phát bản thiết kế xuống code repo.
- 👥 **Ai:** **contract-steward** ratify + **ký hash** contract; **meta** chạy gate & `/sync-design`.
  👤 **Architecture** (+ Security) gác `CONTRACT_READY_GATE`.
- ⚙️ **Diễn ra:** steward tính SHA256 mỗi contract → ghi `tracking/contract-signatures.json` cho mọi
  consumer; mọi consumer ký xong → `/wave-gate-contract-ready` → `SYNC_DESIGN` → `/sync-design` đẩy snapshot
  design xuống `-BOUNDARIES`/`-EXPERIENCES` → stage `PARALLEL_WORK`.
- 🛡️ **Chống sửa ngầm:** hook `contract-hash-check` **re-hash mỗi lần đụng source**; lệch hash → **block**
  (`FM-CONTRACT-DRIFT`). Đổi contract phải bump version + ký lại.
- 💎 **Giá trị:** **một bản hợp đồng bất biến** làm "đường biên" giữa các đội — tiền đề chạy song song mà
  vẫn ghép khít.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu vì sao phải "khoá hợp đồng" xong mới cho code song song
  - [ ] Hiểu hook re-hash: sửa lệch hợp đồng đã ký sẽ bị chặn (`FM-CONTRACT-DRIFT`)
  - [ ] Biết `/sync-design` đẩy gì xuống code repo

### 3.7 — BƯỚC 6: BOUNDARIES + EXPERIENCES — "Code thật, song song"
- 🎯 **Mục tiêu:** hiện thực hoá thiết kế thành code chạy được.
- 🗂️ **Hình hài 2 repo code (khuôn chung):** `-BOUNDARIES` và `-EXPERIENCES` là **2 repo "vỏ"** — mỗi cái
  chứa **nhiều service repo con**; mỗi service (1 boundary backend / 1 experience frontend) là **một git
  repo riêng** theo **cùng một khuôn** (có sẵn folder mẫu `{{BOUNDARY-NAME}}` / `{{EXPERIENCE-NAME}}`).
  Mỗi service repo gồm:

  | Thành phần | Là gì |
  |---|---|
  | `CLAUDE.md` | Router/identity + non-negotiables của service |
  | `Execution/STATE.json` | **State machine LOCAL** của service (không sync ngược SPECS) |
  | `design/` | **Bản chụp chỉ đọc** SPECS đẩy xuống qua `/sync-design`: `MANIFEST.md` (scope wave + allowlist contract + exit criteria + lệnh build/lint/test), `CHARTER.md`, `design/*` (HLD, ADR, workflow), `product/*` (FEAT+AC, BR; FE thêm UX-SPEC, persona, mockup), `contracts/*` (api/ui/event/data đã ký), `_shared/*`, `.sync-manifest.json` (hash audit) |
  | `requirements/<wave>/` | Delta mỗi lần sync (README + MANIFEST + contracts + `decisions.md`) |
  | `tracking/contract-signatures.json` | Chữ ký hash các contract service này tiêu thụ |
  | `scripts/state.py` | CLI điều khiển STATE.json |
  | `.claude/agents/` + `commands/` | 6 agent + 12 slash command (xem dưới) |
  | `.knowledge-graph.yml` | Sổ entity append-only (component/service/decision đã học) |
  | `src/` + `tests/` | Code thật + test (thêm vào lúc dev) |

- 🤖 **6 agent (track kind) mỗi service:** BE = `dev-backend · fix-backend · review-backend · test-unit ·
  test-contract · test-integration`; FE = `dev · fix · review · test-unit · test-component · test-contract`.
  Gọi qua slash: `/spawn-dev`, `/spawn-fix <bug>`, `/spawn-review`, `/spawn-test-*` + lệnh state
  `/stage-transition`, `/wave-rollover`, `/blocker-raise|resolve`, `/state-summary`.
- 🔁 **State machine LOCAL của service:** `IDLE → IMPLEMENTING → READY_FOR_REVIEW → {FIX_BUGS → IMPLEMENTING
  → READY_FOR_REVIEW}* → READY_FOR_TEST → DONE` (+ BLOCKED). `owned_paths = src/** · tests/** ·
  requirements/** · .knowledge-graph.yml` — hook chặn sửa `design/**` (chỉ đọc).
- 🧵 **Vì sao song song:** contract đã khoá ở bước 5 → BE & FE làm **đồng thời** không chờ nhau; test-plan
  chỉ cần contract (chạy được trước cả khi có code). Review **gộp vào PARALLEL_WORK** (v4.5.0).
- 🛡️ **Drift = KHÔNG tự sửa code:** nếu code lệch contract đã ký (hash trong `.sync-manifest.json` không
  khớp) → **không patch code cho khớp**; mở **CR** về SPECS, ký lại, `/sync-design` lại.
- 📒 **Bắt buộc decision log:** mỗi thay đổi code đáng kể phải có commit `# DECISION-REF: ADR-xxx | BR-yyy
  | CONTRACT-zzz` + 1 row `decisions.md`; `decision-log-validate.py` đối chiếu.
- ✅ **Cổng dev-ready của service** (MANIFEST §exit criteria): build green · lint green · test green
  (unit+contract+integration/component) · coverage đạt ngưỡng · mọi FEAT AC pass · 0 test rỗng · chữ ký
  contract còn khớp · decision-log đủ · knowledge-graph cập nhật · không hardcode secret.
- ⚙️ **Diễn ra:** xong gọi `/dev-handoff`, `/review-handoff`; khi **mọi track COMPLETE** →
  `/wave-gate-dev-ready` → stage `QC`. (Aggregator chỉ đòi đúng `expected_kinds` theo class×strategy —
  vd `slice + horizontal-be` chỉ cần `dev-backend + test-unit + test-contract`.)
- 💎 **Giá trị:** **sản phẩm chạy được** cho wave này, ghép vừa khít vì cùng tuân một contract.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu mỗi boundary/experience là **1 git repo riêng** theo cùng khuôn; `design/` là bản chụp **chỉ đọc** từ SPECS
  - [ ] Vẽ được state machine LOCAL của service (`IDLE→IMPLEMENTING→…→DONE`) + biết `owned_paths`
  - [ ] Hiểu vì sao BE & FE làm song song; test-plan chạy được trước cả khi có code
  - [ ] Hiểu **drift → mở CR**, KHÔNG patch code cho khớp contract
  - [ ] Biết cổng dev-ready của service đòi gì (build/lint/test/coverage/decision-log)

### 3.8 — BƯỚC 7: TESTING — "Kiểm thử THẬT & quyết định ship hay không"
- 🎯 **Mục tiêu:** xác minh code đúng nghiệp vụ + đủ chất lượng để ship.
- 👥 **Ai:** **qa-author** viết test case (registry tích luỹ, dedup Jaccard ≥0.7 → REUSE); **qa-executor**
  chạy **thật** (curl/Playwright/k6/axe) → log → tạo bug; **qa-translator** re-map khi AC đổi. 👤 **QA
  người duyệt QC** (≠ Nghiệp vụ) cầm quyền ký duyệt.
- 🚫 **Anti-fake invariant:** QA gọi service **thật**; `check_connectivity.py` chạy trước — service down →
  mark **BLOCKED**, **không** giả vờ PASS. Hook `capture-evidence` lưu stdout/stderr/screenshot làm bằng chứng.
- 🐞 **Bug có định tuyến:** mỗi bug khai `layer` (backend/frontend/integration/…) + `boundaries`/
  `experiences` + `severity` (P1–P4) + `found_in_tc` + `linked_feat`/`linked_br` → dev đúng đội pull đúng bug.
- ⚖️ **Phán quyết ký duyệt (`/wave-signoff`):**

  | Verdict | Khi nào | Hệ quả |
  |---|---|---|
  | **APPROVED** | Không còn P1/P2 OPEN, coverage đạt ngưỡng | ✅ Ship-ready |
  | **CONDITIONAL** | Còn P3 backlog / coverage thiếu | ⚠️ Ship kèm điều kiện (verify wave sau) |
  | **REJECTED** | Còn P1 OPEN / fail exit criteria | ✗ Chặn ship → quay lại fix |

- ⚙️ **Diễn ra:** `/sync-from-domain` + `/sync-from-specs` → `/wave-open N` → chạy test → bug thì
  `/sync-bugs-to-specs` (`SPECS/qc/bugs/`). Người duyệt QC chạy `/wave-signoff` → `/sync-wave-to-specs`
  (`SPECS/qc/waves/WN/` + WAVE-REPORT).
- 💎 **Giá trị:** một **phán quyết chất lượng có thẩm quyền** + hồ sơ test truy vết được.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu "test thật" (anti-fake): service chết thì đánh BLOCKED, không giả PASS
  - [ ] Phân biệt 3 phán quyết: APPROVED / CONDITIONAL / REJECTED
  - [ ] Hiểu vì sao người ký QA bắt buộc khác người Business

### 3.9 — BƯỚC 8: Đóng Wave & ship
- 🎯 **Mục tiêu:** chốt wave, chuyển wave kế.
- 👥 **Ai:** **meta** ở SPECS gọi `/sign-qc`. 👤 **Delivery** xác nhận giao.
- ⚙️ **Diễn ra:** `QC → WAVE_END` ✅. Wave N+1 lặp lại từ Bước 2. Orchestrator quan sát toàn bộ qua
  WebSocket suốt quá trình.
- 💎 **Giá trị:** một **lát release đã kiểm chứng** cộng dồn vào bản release sản phẩm.
- ✅ **Checklist người mới cần nắm:**
  - [ ] Hiểu 1 bản release = nhiều đợt (wave) đã được ký duyệt cộng dồn
  - [ ] Biết `/sign-qc` đóng đợt hiện tại và mở đường cho đợt kế

---

## 4. TRACE 1 FEATURE XUYÊN TOÀN BỘ (ví dụ minh hoạ)

> Lấy một tính năng giả định **"Đặt lịch sửa xe"** để thấy nó chảy qua cả 2 pha. (Ví dụ minh hoạ chung —
> không phải artifact thật trong `_discovery`.)

```
① CON NGƯỜI có ý tưởng
   └─▶ DISCOVERY: D0 hypothesis → D1 persona "Chủ xe" + capability "đặt lịch" (gắn business outcome)
        → D2 event storming → D3 tách boundary "booking" + experience "web-customer" + chốt stack
        → D4 ký contract api/booking → D5 CHARTER → D6 PRD/ROADMAP → D7 WAVE-SEQUENCE (xếp vào Wave 1)
   └─▶ /sync-to-specs  ──▶ SPECS (charters, contracts, WAVE-SEQUENCE, PRD…)
   └─▶ /sync-to-domain ──▶ DOMAIN/_inputs/        ⟵ kết thúc PHA 1, đông cứng

② SPECS: meta /wave-start 1   → PLANNING → DOMAIN (chờ nghiệp vụ)

③ DOMAIN: po-author viết FEAT "Đặt lịch sửa xe" (AC tiếng Việt) + ba-author viết BR
   └─▶ translator dịch → /sync-to-specs ──▶ SPECS/boundaries/booking/product/feats/FEAT-*.md
   └─▶ SPECS: /wave-domain-ready  → ARCHITECT

④ ARCHITECT: sa-author viết HLD + domain-model boundary "booking" + adr-author viết ADR
   (song song) UIUX: ds-author viết tokens + component cho "web-customer"
   └─▶ cả hai /sync-to-specs ──▶ SPECS/.../design/* + SPECS/design-systems/*
   └─▶ SPECS: /wave-architect-ready  → CONTRACT_READY_GATE

⑤ SPECS: contract-steward ratify + ký hash contract  → /wave-gate-contract-ready
   └─▶ SYNC_DESIGN → /sync-design ──▶ đẩy snapshot design xuống CODE REPOS → PARALLEL_WORK

⑥ -BOUNDARIES (backend) + -EXPERIENCES (frontend)  ← code repos (CHƯA tồn tại ở đây)
   nhiều agent SONG SONG: dev-backend, dev-web, review, test-plan… mỗi track register-track về SPECS
   → xong gọi /dev-handoff, /review-handoff
   └─▶ SPECS: /wave-gate-dev-ready (mọi track COMPLETE)  → QC

⑦ TESTING: /sync-from-domain + /sync-from-specs → /wave-open 1
   qa-author viết TC → qa-executor chạy THẬT → bug? → /sync-bugs-to-specs ──▶ SPECS/qc/bugs/
   QC duyệt /wave-signoff APPROVED → /sync-wave-to-specs ──▶ SPECS/qc/waves/W1/
   └─▶ SPECS: /sign-qc  → WAVE_END  ✅ ship Wave 1

⟲ Wave 2 lặp lại từ ②    |    ORCHESTRATOR quan sát toàn bộ qua WebSocket suốt quá trình
```

### Chuỗi truy vết feature → ship (vì sao "có audit toàn trình")

```
GIẢ THUYẾT (D0) → EPIC → FEATURE (AC) + BUSINESS-RULE → CONTRACT (ký hash)
   → HLD/domain-model/ADR + DESIGN-SYSTEM → TEST-CASE (link FEAT+BR)
   → SOURCE-CODE (commit có DECISION-REF) → TEST-EXECUTION thật → BUG (severity, layer)
   → fix → verify → WAVE-REPORT (QA ký) → SHIP
```

→ Bất kỳ dòng code nào cũng **lần ngược được** tới giả thuyết D0 ban đầu. Đây là cốt lõi "hướng sản phẩm":
mỗi mảnh thực thi đều có lý do kinh doanh truy được.

---

## 5. TOÀN BỘ QUY TRÌNH TẠO RA GIÁ TRỊ GÌ? (vì sao phải cầu kỳ thế này)

Nhìn từng bước thì thấy "rườm rà". Giá trị nằm ở **tổng thể**:

1. **Tách concern triệt để** — nghiệp vụ (DOMAIN) / kỹ thuật (ARCHITECT) / giao diện (UIUX) viết ở 3 nơi,
   3 "ngôn ngữ", hook cấm lẫn jargon. → sửa một tầng không vỡ tầng khác; mỗi người đọc đúng phần mình.
2. **Chống drift bằng hợp đồng + hash** — contract ký rồi là bất biến (re-hash mỗi lần đụng). →
   backend/frontend code **song song** vẫn ghép khít.
3. **AI có kỷ luật, không tuỳ hứng** — mọi luật được **hook áp tự động** (chặn edit ngoài `owned_paths`,
   chặn vượt gate, chặn ghi `_inputs`). Agent không "đi tắt" kể cả khi muốn.
4. **Con người giữ quyền ở đúng chỗ** — 5 người duyệt chính ký các cổng; QA buộc ≠ Nghiệp vụ. → AI làm phần nặng,
   người quyết phần rủi ro cao.
5. **Truy vết toàn trình** — `decisions.md` append-only, frontmatter audit, contract hash,
   `.sync-metadata.json`. → mọi dòng code lần ngược tới giả thuyết D0.
6. **Song song hoá có kiểm soát** — nhờ contract khoá + state machine, nhiều trạm chạy đồng thời nhưng
   SPECS vẫn biết tổng tiến độ; orchestrator phát sóng theo thời gian thực.
7. **Lặp lại bền vững theo wave** — cùng một khuôn chạy đi chạy lại cho W1, W2… nên chất lượng & tốc độ
   ổn định; người mới học **một** wave là làm được mọi wave.

**Tóm một câu:** ADLC biến "code bằng AI" — vốn dễ loạn — thành một **dây chuyền có hợp đồng, có cổng, có
audit**, để release ra đời **nhanh mà vẫn đúng nghiệp vụ, nhất quán kỹ thuật, kiểm chứng được**.

### FAQ người mới hay hỏi
- **"Tôi sửa 1 chữ ở FEATURE thì ai bị ảnh hưởng?"** → tra `SPECS/DOC-DEPENDENCY-MAP.md §3` (propagation
  theo tier T0→T5).
- **"Vì sao DOMAIN không được viết 'endpoint'?"** → để `product/*` thuần nghiệp vụ; chi tiết kỹ thuật là
  việc ARCHITECT. Hook `lint-frontmatter.py` chặn.
- **"Contract đã ký mà cần đổi thì sao?"** → mở **CR**, theo mức độ sẽ cần người duyệt tương ứng;
  không sửa lén (hash sẽ tố cáo). Xem §8.
- **"BOUNDARIES/EXPERIENCES đâu?"** → DriverPlus chưa tạo — dự án đang ở đầu nguồn (DISCOVERY); orchestrator
  báo 2 repo "chưa có". Cấu trúc của chúng — xem §3.7.
- **"Muốn xem mọi thứ đang ở đâu?"** → bật orchestrator (§7.2 thực hành), đọc trạng thái trực tiếp.
- **"Tôi mới join — bao giờ được nhận task thật?"** → đi theo **checklist "người mới cần nắm"** ở cuối từng
  phase (§2 cho Discovery D0–D7, §3 cho execution). Tick đủ checklist của phase nào thì xin **task nhỏ có
  người kèm** ở phase đó; làm tốt vài lần rồi nhận task độc lập. Việc **rủi ro cao** (đụng hợp đồng đã ký,
  cross-boundary, cần `--force`) thì luôn hỏi người duyệt trước (§8).

---

# ═══════════ PHẦN B — CƠ CHẾ KỸ THUẬT (TRA CỨU) ═══════════

> Phần này là *cẩm nang kỹ thuật* phía sau câu chuyện sản phẩm — đọc khi cần hiểu cách hệ thống được
> kiểm soát tự động. Nắm cơ chế trạng thái và hook là đủ để tra cứu phần còn lại.

## 6. TRẠNG THÁI & HÀNG RÀO KIỂM SOÁT (vì sao hệ thống không "loạn")

Mọi kỷ luật được áp bằng **hook** (Claude Code) chạy tự động, không dựa vào agent tự giác:
| Hook | Cung cấp |
|---|---|
| `SessionStart` | In STATE summary + NON-NEGOTIABLES đầu session |
| `UserPromptSubmit` | Inject `[STATE: W{N}/{stage}/agent=...]` mỗi turn |
| `PreToolUse boundary-block` | Chặn edit ngoài `owned_paths` |
| `PreToolUse meta-edit-block` | Chặn meta edit ngoài allowlist (meta không viết artifact) |
| `PreToolUse readonly-inputs` | Chặn ghi `_inputs/**` |
| `PreToolUse contract-hash-check` (SPECS) | Re-hash signed contract; drift → block (`FM-CONTRACT-DRIFT`) |
| `PreCompact` | Pin STATE + 3 decision mới nhất vào compaction summary |

Cộng thêm: `*-gate.py` chặn vượt stage khi chưa đủ exit gate; `lint-*.py` chặn sync khi sai
frontmatter/jargon; `denylist` trong `_routing/*.yaml` chặn ghi nhầm territory repo khác.

---

## 7. LỘ TRÌNH HỌC + TRA CỨU

> 📌 Checklist "người mới cần nắm" đã nằm ở **cuối mỗi phase** (§2 cho Discovery D0–D7, §3 cho execution).
> Mục này chỉ còn **lộ trình đọc** + **bảng tra cứu nhanh**.

### 7.1 Nguyên tắc đường tắt
1. **Hệ thống FRACTAL** — học kỹ 1 repo phase, 4 cái còn lại đọc lướt theo cùng khuôn.
2. **State machine là xương sống** — hiểu `_stage_flow` trong mỗi STATE.json là hiểu luồng điều khiển.
3. **Sync scripts là khớp nối** — đọc docstring + allowlist/denylist đầu mỗi `sync-*.py`.
4. **Học bằng cách CHẠY** — bật orchestrator, xem trạng thái trực tiếp, `--dry-run` còn nhanh hơn đọc code.

### 7.2 Lộ trình ĐỌC HIỂU (≈1.5–2 ngày)
**Bước 1 — Tổng quan (1h):** §0–§3 tài liệu này → `SPECS/DOC-DEPENDENCY-MAP.md §1-§3` (tier T0→T5 +
propagation) → `DISCOVERY/DISCOVERY-RESPONSIBILITIES.md` (bảng tổng quan).

**Bước 2 — Học khuôn qua DOMAIN (3h):** `DOMAIN/CLAUDE.md` → `DOMAIN-RESPONSIBILITIES.md` →
`Execution/STATE.json` (`_stage_flow`, `_stage_enum`) → 3 agent spec trong `.claude/agents/` → đầu
`scripts/sync-to-specs.py` + `_routing/DOMAIN-TO-SPECS-MAP.yaml`. ✅ Xong = hiểu khuôn cho ARCHITECT/UIUX/TESTING.

**Bước 3 — Đọc lướt 3 repo (2h, mỗi repo 40', chỉ tìm ĐIỂM KHÁC):**
- ARCHITECT: pull `sync-from-*` (anti-drift), lint cấm product-jargon.
- UIUX: artifact `tokens.json`/components, Figma importer STUB.
- TESTING: `registry/` (pool tích lũy) + `waves/` + chạy thật + ký duyệt.

**Bước 4 — SPECS hub sâu (2h):** `SPECS/CLAUDE.md` + `Execution/STATE.json` `_stage_flow` + NON-NEG #6
(3 luồng sync) + `.sync-metadata.json`.

**Bước 5 — Hands-on (2h):**
```bash
# Bật orchestrator (đọc trạng thái trực tiếp cả 8 repo)
cd DRIVERPLUS-ADLC-ORCHESTRATOR/backend && python3 -m venv .venv && \
  .venv/bin/pip install -e . && \
  ADLC_ADLC_ROOT=/Users/all_engineer/Projects/nambp \
  .venv/bin/uvicorn app.main:app --port 8765 --reload
# Terminal 2: cd ../frontend && npm install && npm run dev → http://127.0.0.1:5173

# Đọc state + dry-run (an toàn, không ghi thật):
python3 DRIVERPLUS-ADLC-DISCOVERY/scripts/state.py summary
python3 DRIVERPLUS-ADLC-DOMAIN/scripts/sync-to-specs.py --dry-run
```
> ⚠ `ADLC_ROOT` mặc định `~/srcroot/ADLC/...` — **phải override** `ADLC_ADLC_ROOT=/Users/all_engineer/Projects/nambp`
> thì orchestrator mới thấy đúng 7 repo (và sẽ báo BOUNDARIES/EXPERIENCES "chưa có" — đúng, vì chưa tạo).

### 7.3 Bảng tra "muốn hiểu X → đọc file nào"
| Muốn hiểu | Đọc |
|---|---|
| Luồng điều khiển 1 wave | `SPECS/Execution/STATE.json` → `_stage_flow` |
| Một repo làm gì | `<repo>/<PHASE>-RESPONSIBILITIES.md` |
| Turn này AI đóng vai gì | `<repo>/CLAUDE.md §3 ROLE ROUTING` |
| Dữ liệu chảy đi đâu | `<repo>/scripts/sync-*.py` + `_routing/*.yaml` |
| Luật cấm / hàng rào | `<repo>/CLAUDE.md §1 NON-NEGOTIABLES` + `scripts/hooks/` |
| Ai propagate vào ai khi sửa doc | `SPECS/DOC-DEPENDENCY-MAP.md §3` |

---

## 8. NGƯỜI DUYỆT + MỨC ĐỘ THAY ĐỔI (con người ký duyệt)

### 8.1 Năm người duyệt chính
| Vai trò | Người | Gác/ký |
|---|---|---|
| Kiến trúc | thangnvc | stack (D3), HLD/ADR, `CONTRACT_READY_GATE` |
| Giao hàng | nambp.os | WAVE-SEQUENCE (D7), `/wave-start`, `/sign-qc` |
| Nghiệp vụ | dangbh | giả thuyết/persona/năng lực (D0–D1), `/wave-domain-ready` |
| QA | datnt (**phải ≠ Nghiệp vụ**) | `/wave-signoff` (APPROVED/CONDITIONAL/REJECTED) |
| Bảo mật | ducpdd | bảo mật (D3 stack, ARCHITECT), review trước DEV |

### 8.2 Mức độ CR — ai duyệt mức nào
Thay đổi sau khi đã đông cứng phải đi qua **yêu cầu thay đổi (CR)** theo mức nghiêm trọng:

| Mức độ | Ví dụ phát sinh | Người duyệt | Lan toả |
|---|---|---|---|
| **COSMETIC** | Sửa lỗi chữ, định dạng | Sửa trực tiếp (không cần CR) | Không |
| **MINOR** | Thêm field không bắt buộc, làm rõ nội dung | Tác giả / người duyệt liên quan | T2–T5 |
| **MODERATE** | Đổi hành vi, contract thêm field, chỉnh CHARTER, ADR mới | Người duyệt liên quan (Kiến trúc cho contract; Nghiệp vụ cho scope) | T1→T2, ký lại contract |
| **MAJOR** | Đổi scope (thêm/bớt epic/feature/boundary), event schema breaking, đổi stack | **Nghiệp vụ + Kiến trúc** | T0 inventory, tạo lại wave/MANIFEST |
| **CRITICAL** | Đổi policy T0 (`ADLC.md`, `ARCHITECTURE-PRINCIPLES.md`, `DOC-DEPENDENCY-MAP.md`), đại tu stack | **Tất cả người duyệt** (đủ quorum) | Kiểm lại toàn dự án |

> Chi tiết tầng lan truyền T0→T5 + bảng người sở hữu từng tài liệu: `SPECS/DOC-DEPENDENCY-MAP.md §6–§7`.

---

> 🔚 **Hết.** Người mới: đọc PHẦN A là đủ để hiểu "một feature ra đời thế nào" và "ai làm gì". Khi cần đào
> sâu cơ chế kiểm soát → PHẦN B. Học tới đâu, tick **checklist "người mới cần nắm"** ở cuối mỗi phase (§2, §3)
> tới đó. Bí → §7.3 bảng tra + §8 hỏi người duyệt.
