# asgard-fde-cli

[English](README.md) | **繁體中文**

Asgard FDE 的命令列工具（`asgard-cli`）。

## 安裝

macOS 或 Linux 上一行指令：

```bash
curl -fsSL https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.sh | sh
```

Windows 上，在 PowerShell 裡：

```powershell
irm https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.ps1 | iex
```

`install.ps1` 是 Windows 版，做的事一樣：對著 release 自己的 checksums 驗過、裝到 `%LOCALAPPDATA%` 底下、把那個路徑加進使用者的 PATH。裝在使用者自己擁有的地方不需要提權，之後 `asgard-cli update` 也因此能就地換掉它。

它拿這個平台最新的 release、對著那個 release 自己的 checksums 驗過、裝到 `/usr/local/bin`，然後把 binary 跑一次。macOS 會在第一次執行時掃描剛寫下來、沒 notarize 的 binary，所以讓這次掃描發生在安裝時，而不是在客戶面前。跑的是 repo 根目錄的 `install.sh`，把它 pipe 進 shell 之前可以先讀一遍。

Linux 上也刻意裝到 `/usr/local/bin`。`.deb` 和 `.rpm` 裝到 `/usr/bin`，那是套件管理員的目錄，在那裡的 binary 不能換掉自己，`asgard-cli update` 會拒絕，以免留下「dpkg 記的是這版、磁碟上是另一版」的狀態。檔案系統標準把 `/usr/local` 保留給套件管理員以外裝的軟體，所以用這條裝的之後可以自己更新。

下面是同一件事的手動版。

網址不帶版本號，所以跨版本一直有效。GitHub 會把
`/releases/latest/download/<檔名>` 解析到最新的 release：

```bash
curl -fsSL https://github.com/asgard-ai-platform/asgard-fde-cli/releases/latest/download/asgard-cli_darwin_all.tar.gz \
  | tar xz asgard-cli
sudo install -m 0755 asgard-cli /usr/local/bin/asgard-cli
asgard-cli doctor          # 告訴你 helm 在不在 PATH 上
```

把 `darwin_all` 換成 `linux_amd64`、`linux_arm64` 或 `windows_amd64`。
`darwin_all` 同時支援 Intel 與 Apple silicon，所以 Mac 上不用選，
雙擊 `.pkg` 裝的也是同一個 binary。

想讓它自己判斷平台，或是要特定版本而不是最新的：

```bash
repo=asgard-ai-platform/asgard-fde-cli
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
# 一個 binary 同時服務兩種 Mac；其他平台是分架構的。
[ "$os" = darwin ] && arch=all

gh release download --repo "$repo" --pattern "*_${os}_${arch}.tar.gz" \
  --output - | tar xz asgard-cli
sudo install -m 0755 asgard-cli /usr/local/bin/asgard-cli
asgard-cli doctor          # 告訴你 helm 在不在 PATH 上
```

這條也是拿最新的 release，因為 `gh release download` 不給 tag 時就是這個行為。

Debian / Ubuntu 用 `.deb` 一行就好；RPM 與 Alpine 用同一個 release 裡的 `.rpm` 或 `.apk`，做法一樣：

```bash
arch=$(dpkg --print-architecture)      # amd64 或 arm64
curl -fLO https://github.com/asgard-ai-platform/asgard-fde-cli/releases/latest/download/asgard-cli_linux_${arch}.deb
sudo dpkg -i asgard-cli_linux_${arch}.deb
```

這會裝到 `/usr/bin`，所以升級要走 dpkg，不能用 `asgard-cli update`；在那裡跑 update，它會這樣告訴你。想讓工具自己保持最新，就用 tarball 或上面的安裝腳本。

### macOS 可能卡住或殺掉第一次執行

這些 binary 只有 ad-hoc 簽章（Go 的 linker 為了讓它們能執行而加的），沒有經過 notarization，所以 macOS 會掃描第一次執行。同一個資產、同一台機器、都用 `gh` 下載，三種情況都出現過：立刻跑、卡了好幾分鐘才跑、以及被 exit 137 殺掉且零輸出、下一次才正常。瀏覽器下載的一定會失敗：它加上的 quarantine 標記每次都造成 exit 137。

所以如果它看起來什麼都沒做，原因是這個，不是工具壞了：

```bash
xattr -d com.apple.quarantine ./asgard-cli   # 只有瀏覽器下載的才需要
./asgard-cli version                          # 然後再試第二次
```

在會議之前先裝好。

如果你本來就會編 Go，module 直接可用：

```bash
go install github.com/asgard-ai-platform/asgard-fde-cli/cmd/asgard-cli@latest
```

`asgard-cli version` 報的是 release 編進去的值；不帶 ldflags 的 `go build` 會退回 module 與 VCS metadata，不會宣稱一個它沒有的版號。

### 保持在最新版

```bash
asgard-cli update              直接更新到最新的 release
asgard-cli version --check     只問有沒有更新版，什麼都不改
```

每個可能碰到網路的指令都會檢查有沒有更新的 release，每兩小時最多一次，有的話在 stderr 印一行。結果記在 profiles 旁邊的 `update-check.json`，不記進客戶的 repository（那是別人的 checkout，而且會被 commit）。這個檢查跟指令並行，不擋在指令前面，所以真的發出請求的那一次不會比讀快取慢。

`update` 會就地換掉這個 binary：抓這個平台不帶版號的 asset、用雜湊（不是檔名）對著那個 release 自己的 checksums 驗過、在原地先執行新的 binary，確認它能跑之後才 rename 蓋過舊的。這個順序保證安全：被 macOS 殺掉的 build、下載到一半的檔、空的壓縮檔，都會在還沒動到任何東西時就失敗，所以結果只會是新版或原本那版，不會留下一個跑不起來的 binary。在 macOS 上，Gatekeeper 的掃描也在這一步完成，不會發生在客戶面前。

遇到不能處理的情況它會拒絕，不會猜，而且每種拒絕都說該跑什麼：套件管理員裝的由套件管理員更新、寫不進去的目錄要 `sudo asgard-cli update`、`go build` 出來的 binary 沒有可以比對的 release。每個指令印的那一行針對的是你這個安裝該跑的指令。

Windows 上是兩次 rename。執行中的 `.exe` 不能被寫也不能被刪，但可以改名，所以舊的先搬到 `asgard-cli.exe.old`，新的接手原本的名字。被搬開的檔要等執行它的 process 結束才刪得掉，所以在下一次執行時才清掉。更新完在 binary 旁邊看到 `.old` 是正常的，不是失敗。

沒有人叫它，它不會自己更新。在背景覆寫自己的 CLI 必須挑一個時機，而每個時機都可能不對：指令跑到一半、會議中途，或是套件管理員還認為那個檔歸它管。

stderr 不是終端機時更新檢查是關閉的，所以 CI 的 log 與被導向管線的 stderr 不會收到訊息，也不會發出任何請求。說明裡寫著不碰網路的指令永遠不檢查，`init`、`size`、`guide` 都是：它們要在會議上、沒有網路時也能回答，而一個失敗時不出聲的請求仍然會從會議現場的網路發出去。其他情況用 `ASGARD_NO_UPDATE_CHECK` 關掉背景檢查；`--check` 仍然會回答。

repository 本身也能說出版本，完全不需要網路：`.asgard-scaffold.json` 記了這個 CLI 送出的每個檔案是哪一版寫的，所以一個已經被更新版跑過的 checkout，跑 `asgard-cli gate` 就會印出那個版號。

## 開發

```bash
make build       # .out/asgard-cli
make install     # 裝到你的 PATH 上
make gate        # CI 檢查的每一項
make help        # 其餘的
```

`make install` 就是 `go install ./cmd/asgard-cli`：binary 會放在 `GOBIN`，沒設的話放在 `go env GOPATH`/bin，裝完會印出路徑；如果 PATH 上有更前面的同名檔會遮住它，也會警告。要裝到別的地方就指定：

```bash
make install GOBIN=~/.local/bin
```

哪個目錄放什麼寫在 [STRUCTURE.md](STRUCTURE.md)，這裡不重複，避免兩份目錄清單互相漂移。

要新增子指令：在 `internal/cli/` 寫一個 `newXxxCmd()`，並在 `root.go` 用 `addTo(cmd, group..., ...)` 註冊。分組是必填的：cobra 對父層沒有的 `GroupID` 會 panic，所以每個指令加進來時都必須決定它屬於哪一組。

- [Goal.md](Goal.md) —— 這個工具是為誰、做什麼，四點。
- [APPROACH.md](APPROACH.md) —— 主要功能怎麼實作的：語料、指標的形式、稽核、檢索、`init` 寫了什麼。
- [STRUCTURE.md](STRUCTURE.md) —— 每個目錄是做什麼的，包含各份內嵌語料以及一個改動屬於哪一份。
- [AGENTS.md](AGENTS.md) —— 這個 repo 遵循的慣例，以及 gate 是什麼。
- [TASK.md](TASK.md) —— 還沒完成的部分。

## Coding Agent 會怎麼用它

這一節不在英文版裡，它是給正在盤點的人看的。

這支 CLI 有兩種讀者，主要都不是坐在終端機前打字的人。

第一種是 FDE 本人，通常在客戶會議前後：`asgard-cli init` 在空目錄就會把整份語料寫出來，`guide` 和 `size` 也一樣，都不需要 repo、網路或登入，因為這些問題是在會議室裡被問的，那時候還沒有目錄。

第二種、也是設計時主要針對的，是在客戶 repo 裡工作的 Coding Agent。它的流程是：

```
1. asgard-cli init             一次把骨架、綁定、平台參考素材都建好
2. 讀 .agents/skills/          asgard-cr-shapes（這座叢集的 CRD 欄位）
                               asgard-workflow-processors（這座 runtime 的 processor config）
                               asgard-cr-verification（平台會怎麼檢查、每個規則碼是什麼意思）
3. asgard-cli add <kind>       寫一份 CR 骨架，會失敗得無聲無息的部分是對的
4. asgard-cli gate             本機能檢的全部，一個指令
5. git push <tag>              推上去
6. asgard-cli pipeline runs watch   讀回平台的 plan report
```

其中三件事是刻意這樣設計的：

- 第 4 步是一個指令，不是一張清單。一個全是 CR 的 repo 沒有編譯器；寫編譯語言的 agent 不會漏檢查，是因為不管改了什麼都要跑 build，而 build 是一個指令。散在 `AGENTS.md`、提示文、agent 記憶裡的清單會過期，也確實過期過。
- 第 2 步的素材是抓下來的，不是編在 binary 裡的。客戶的 server 版本跟他裝的 CLI release 無關（On-Prem），所以某個欄位叫什麼，取決於那座 server，而不是 CLI 版本。
- 第 4 步不重做第 6 步的檢查。CR 能不能被接受由 apiserver 決定，而平台永遠不發叢集憑證給 client。本機綠燈表示值得推上去，不表示會部署成功。

每一個讀取類指令都接受 `--format json`，因為 agent 需要解析輸出。`check` 與 `verify` 這一對最重要：文字模式下警告與失敗只差左邊一個字，但只有失敗是致命的；JSON 模式下它們是兩個不同的陣列。

## 指令總覽

```
Ask —— 平台是什麼、一個形狀怎麼組起來
  這一半不是指令，是檔案。asgard-cli init 會把全部寫進
  .agents/skills/asgard-platform/，用 cat 和 grep 讀：
    wiki/       平台由什麼構成，每一塊是給誰的
    usecase/    一種部署形狀怎麼一個欄位一個欄位組起來
    needs/      這個形狀開工前要先跟客戶拿到什麼
    brief/      你正要做的事，前人在哪裡做錯過
    guide/      一個決策，以及反悔的代價
    aliases.md  客戶說的話 -> 該搜什麼字

  guide [name]           一個決策的指引，會對著這個 repo 的現況讀
  size [shape]           一個能力寫出來之前，它由什麼構成、要多少
  issue-report           這支工具哪裡錯了或不知道，怎麼回報

Build —— 寫出 repo 與裡面的 CR
  init                   一個指令完成 onboarding：骨架 + 綁定 + 參考素材
  project                每份 chart 宣告了什麼
    add <slug>           寫一個 project 的 chart 骨架
  add <kind> <name>      寫一份 CR 骨架，接上 chart 已宣告的東西
  question / add / answered      沒人回答的問題，以及答案是什麼
  request / add / target / ready / done    客戶要的東西
  task / add / ready / start / done        task spec
  decision add           寫一份帶日期的決議紀錄
  reference add          歸檔客戶自己的素材，連同出處
  local-env              開一頁表單，讓握有憑證的人自己填進 .env

Check —— 這台機器能檢查的全部
  gate [release ...]     ★ 一個指令跑完下面所有能跑的
  check [project]        repo 的結構不變量
  render <release>       用原生 helm 渲染，注入佔位的平台值
  verify [release]       渲染後的 CR 交叉引用與不變量
  doctor                 外部工具在不在，不在的話怎麼裝

Deploy —— 平台，以及它知道的事
  login / logout / whoami
  profile    list / show / set / remove —— 這支 binary 沒有內建的那些安裝
  workspace  list / use <id> / show
  pipeline   connect / connections / repos / create / list / use <id> / show
             projects / release create|show|update|destroy|detach
             releases / deliveries
             runs list|get|log|watch|approve|reject|cancel
             variables list|set|unset|sync-declared
             manifest
  skill      status / update

不分組 —— 關於這個 binary 而不是關於某個 engagement
  version [--check]      這個 build 是什麼，以及有沒有更新的 release
  update                 就地換成最新的 release

（另有隱藏指令 audit-material，是給維護這份語料的人用的，不是給客戶 onboarding 用的。）
```

## 指令

### `init`

把 repo 骨架寫在這裡，好讓 coding agent 接手。

```bash
mkdir acme-asgard-kube && cd acme-asgard-kube
asgard-cli init
```

```
This writes the Asgard repository skeleton into

    /path/to/acme-asgard-kube

and the repository will be called acme-asgard-kube, after that directory.

Write it here? [Y/n]

This is not a git repository yet. ...
Run `git init` here? [Y/n]

  created      .agents/skills/asgard-fde-onboarding/SKILL.md
  ...
45 created in /path/to/acme-asgard-kube

note: there is no `origin` remote here yet. Connecting derives the provider
      account and the repository from it, so the first thing you will be asked
      for is this repository's remote URL.

Now open this directory in your coding agent and say:

    Connect this repo to the Asgard platform
```

這是唯一寫給人的指令，也是唯一會問問題的。這個工具的其他部分都是寫給在既有 repo 裡工作的 coding agent，而在這個指令跑完之前，repo 還不存在：沒有 `AGENTS.md`、沒有 `CLAUDE.md`、沒有 `.agents/skills/`。在空目錄裡打開的 agent 對 Asgard 一無所知，所以不能要求它先去跑一個需要 workspace id 的指令。

它不碰網路、不需要帳號。骨架只取決於這個工具，跟任何平台無關，所以在飛機上、workspace 還沒開、還沒有人登入時都寫得出來，因此它可以當第一個指令。

把 checkout 接上平台刻意不包含在內。登入、選 workspace、建 pipeline、抓描述 server 的素材，都在之後，由這個指令剛設定好的 agent 帶著做，比讓人照著指令清單手動做好。`asgard-cli gate` 隨時會說還缺什麼。

CLI 有更新、或新增了 project 的時候就再跑一次：既有檔案不動、報成 skipped。`--force` 取用比較新的出貨素材（會丟掉你對骨架的修改）；被這個工具寫入過的檔案（索引、open-questions 表、living spec）兩種情況都會保留並回報。`--yes` 什麼都不問，stdin 不是終端機時行為相同，所以 agent 或 CI 重跑不需要互動。

它拒絕寫進家目錄或檔案系統根目錄，以免整份骨架寫錯一層目錄。

它寫的是每個 engagement 都一樣的那部分：

| | |
|---|---|
| `AGENTS.md` | 平台契約，客戶特有的段落標成 TODO |
| `docs/` | 四層模型（meeting-notes / decisions / living spec）與 SDD 規則 |
| `requirements/` | task 與 request 的索引 |
| `.agents/skills/` | 對任何 Asgard 都成立的設計期 skill，`db-query` 是其中之一；描述某一座 server 的那些來自 `asgard-cli skill update` |
| `assets/` | runtime skill 目錄 |
| `.asgard-pipeline.yaml` | 部署宣告，每個 project 一個 release 待填 |
| `projects/<slug>/` | 每個 project 一份 chart 骨架 |

它不寫客戶自己的知識：有哪些系統、工作怎麼切分、CR 長什麼樣。那些由 onboarding 產出，模板生不出來。

產生出來的骨架第一次跑就能通過自己的 gate：

```bash
asgard-cli gate
```

不要手動跑 `helm lint`。平台會在每次渲染時注入保留的 `.Values.asgard` 區塊，而 chart 不可以在自己的 `values.yaml` 宣告它，所以不帶 `-f` 直接跑，會在每個讀 `.Values.asgard.projectEnvironmentId` 的 chart 上失敗，也就是每個會貼 label 的 chart。`gate` 只供給那一個檔案，其他都不給。

### `project add`

在 `projects/<slug>/chart/app` 底下寫一個 project 的 chart 骨架。

```bash
asgard-cli project add internal
```

一個 project 就是一份 Helm chart。哪些 Release 部署它、部署到哪個平台 Project，是在 `.asgard-pipeline.yaml` 裡宣告的。這個指令只寫 chart，替它宣告 Release 不在這裡做。

這個 project 沒有另外記錄在任何地方。它存在是因為目錄存在、而且宣告檔指名了它的 chart；沒有第三份清單要同步，也就不會有不一致。既有檔案不動，所以重跑是安全的。

slug 會出現在 chart 渲染出來的物件名稱裡，所以要短：Kubernetes 的名稱上限是 63 字元，而衍生出來的名稱會繼承它的長度。

### `guide`

`guide` 對著你所在的 repo 讀一個決策的指引。每份指引不需要 repo 的部分跟其他材料一樣會寫成檔案；指令額外加上的是這個 repo 自己的狀態：有哪些 project、還有什麼沒回答。

```bash
asgard-cli guide                  # 全部的指引，依名字
asgard-cli guide requirements     # 其中一份，對著這個 repo 讀

cat .agents/skills/asgard-platform/guide/requirements.md   # 不需要 repo 的那一半
```

### 這次 engagement 自己那些系統的連結

給夥伴看的 deck 大多是連結，而需要的 id 都已經在磁碟上：`.asgard-cli.yaml`
記著 workspace，git remote 記著 repository。手工組連結容易出錯，而且錯的版本看起來都正常：
給站台首頁而不是正在講的那一頁、在已經是連結的名字旁邊再貼一次網址、因為猜對方打不開就把
連結刪掉。

```bash
asgard-cli links                  # 這份 checkout 綁到哪些東西
```

它只印它知道的。Console 跟 API 是兩個不同的 host，無法互相推出，所以
Console 只有在官方託管的 profile 上才知道；pipeline 的 Console 路徑沒有任何來源記錄過。
這些會印成「沒印，因為……」，不會印一個看起來像的網址。它不碰網路，也不需要 session。


沒有任何指令會報告這個 engagement 進行到哪裡，這是刻意的。`project`、`question`、`request`、`task` 各讀一個檔案，沒有一個會從其他幾個推導出進度。

```
  requirements   把客戶說的話變成一個 request
  projects       決定工作怎麼切成 project
  data-sources   接上客戶的資料庫
  read-path      決定每個 project 的讀取路徑
  entry-point    決定每個 project 的入口
  knowledge      決定非結構化知識放哪裡
  verify         跑驗收 gate
  deploy         部署
  enhance        在已經上線的 repo 上加一個能力
  idle           手上沒有進行中的事
```

這些不是依序抵達的步驟。onboarding 不是線性的：這支工具是從一個 engagement 發展出來的，那個 engagement 裡最貴的三個決策都是做了、建了、又推翻的；一個需求已經全部問完的 engagement 也沒有所謂的階段。所以不會有東西主動把指引推給你，要用 `guide` 依名字讀，或在 `guide/` 裡依主題 grep。

`read-path`、`entry-point`、`knowledge` 會把錯的答案印在對的答案旁邊。這三個是那個 engagement 做錯又推翻的決策，每次錯的都是看起來理所當然的那個。

一份 chart 不一定以入口結尾，這裡也沒有指令會判斷它完成了沒有。一個沒掛任何東西的 SemanticLayer，可能是一份做完的 Mimir 交付物，也可能是還沒人寫的 agent，檔案本身分不出來。`asgard-cli size <shape>` 列出一個形狀由什麼構成，讓人自己比對；沒有任何地方記錄一份 chart 打算長成什麼樣，因為原本打算建什麼不是這支工具能檢查的。

### `project`、`request`、`task`、`question`

四個指令把 repo 讀回來給你，一個指令一個檔，每個都接受 `--format json`。它們不從彼此推導任何東西。

```bash
asgard-cli question    # 沒人回答的問題，以及每一個要問誰
asgard-cli request     # 客戶要的、還沒做完的
asgard-cli task        # 開著的 task spec
asgard-cli project     # 每份 chart 宣告了什麼
```

先讀 `question`。在別人開的 repo 裡，繞過一個他們早就知道還沒解決的問題去設計，是最容易造成傷害的做法。

```
Projects:

  insight              DataConnector, SemanticLayer
  helpdesk             chart is empty
```

它只列出每份 chart 有什麼，不列出缺什麼。以前會對照每個 project 記錄的「形狀」來報告缺什麼，但報出「這個形狀要 X 而 X 不在」，等於把某人記下的意圖當成這支工具能檢查的規格。清單直接來自 repo（宣告檔指名的 chart 路徑，加上 `projects/` 底下的目錄），所以沒有第二份會漂移。

### `reference` —— 歸檔客戶交過來的東西

    asgard-cli reference add <file> --what "<這是什麼>" \
      --from "<誰給的>" --dated <文件自己的日期>

`references/` 是給人和寫 spec 的 agent 看的背景，不是執行中的 agent 讀的東西。
agent 在 runtime 需要的領域知識屬於 skill，因為 skill 會同步進平台，這個目錄不會。

每個案子都會歸檔文件，但做法各不相同，每個都自己發明一套出處表，其中一個發明的目錄名後來
被當成了慣例，所以有這個指令。它逐位元組複製檔案，之後的版本可以跟歸檔的那份 diff；出處寫進
`references/_index.md`，不會在客戶自己的檔案裡加 header。

`--dated` 是那份文件自己的日期，不是今天，因為素材是否過期取決於文件日期。沒有日期的文件，
就記下它沒有日期。`asgard-cli check` 會對欄位不全的列提出警告。

### `local-env` —— 一頁表單,因為密碼不能進 transcript

    asgard-cli local-env
    asgard-cli local-env --focus UOF_DB_HOST,UOF_DB_PASSWORD

coding agent 不可以叫任何人把密碼告訴它，不管是在對話裡，還是「你貼上來我之後刪掉」：
進過 transcript 的憑證就算已經外洩。原本的替代做法是叫一個可能不是工程師的人去打開
dotfile、找到那一行、還要注意空白，這個要求常常失敗。

所以 agent 先把要的 key 名稱寫進去、值留空，再用這個指令開一頁表單讓人填：127.0.0.1 上
一個隨機 port、URL 帶一次性 token、不回應任何其他 host 名稱，而且那一頁只能跟服務它
的那個 process 溝通。表單存檔後就關閉。

回傳的只有 key 的名稱，不會有值，存檔時、錯誤訊息和摘要裡都一樣。
`--focus` 會標出你在等的 key，但刻意不隱藏其他的：填表的人可能知道一個還沒有人提過的
資料庫，也可以自己加 key，所以之後要重讀 `.env`，不要假設拿回來的只有你問的那些。

### `request`、`task`、`question`、`decision` —— 寫紀錄

工作以 request 進來：客戶想要、而 agent 今天做不到的一件事。其他所有東西都掛在它下面。

```bash
asgard-cli request add "倉管人員想在聊天裡問庫存"
asgard-cli request target REQ-001 erp
asgard-cli request ready REQ-001

asgard-cli task add "把庫存查詢開出來" --request REQ-001 --project erp --complexity M
asgard-cli task ready TASK-001
asgard-cli task start TASK-001
asgard-cli task done TASK-001

asgard-cli question add "哪一個庫存數字才算數" --blocks REQ-001 --ask "倉管主管"
asgard-cli question answered 1 "只算 608 儲位" --decision 2026-09-04-safety-stock.md

asgard-cli decision add "官網一律走固定查詢工具讀取" --module architecture.md
```

這些狀態都不存在 CLI 裡。每個指令都在客戶的 repo 裡寫檔，因為下一個 agent 會打開的是那個 repo：

| 紀錄 | 檔案 | 帶什麼 |
|---|---|---|
| request | `requirements/requests/REQ-xxx-<name>.md` ＋ registry 的一列 | 提出日期、狀態、目標 project、客戶自己的用字 |
| task spec | `requirements/tasks/TASK-xxx-<name>.md` ＋ queue 的一列 | 建立日期、狀態、SDD 各節、每次狀態轉移的日期紀錄 |
| open question | `docs/open-questions.md` 裡的一列 | 提出日期、它擋住什麼、誰能回答 |
| decision | `docs/decisions/YYYY-MM-DD-<topic>.md` ＋ 一列追溯 | 檔名裡的日期、它改動的模組 |

這些做成指令，而不是「請寫一個檔」的指示，是因為每筆紀錄都存在不只一個地方。一個 task 的狀態在 queue 表格裡、在 spec 自己的 `Meta` 裡、也在 spec 的執行紀錄裡；一個 request 的目標 project 在 registry 的 Spec 欄與它的 `Meta` 裡。手動改一次要動三到四處，而其中兩處互相矛盾時，下一個讀的人無法判斷哪個是現況。這裡每個指令都一次改完全部，日期由指令填入，不用問。

### `add`

在一個 project 的 chart 裡寫一份 CR 骨架，其中會無聲失敗的部分已經寫對。

```bash
asgard-cli add                                   # 列出所有 kind
asgard-cli add dataconnector erp --db-class postgres --project erp
asgard-cli add flowagent support --bot-class line --project site
```

十種 kind：`dataconnector`、`semanticlayer`、`agent`、`httptool`、`querytool`、`skillset`、`trigger`、`knowledgedrive`、`plugin`、`flowagent`。

它產生的是骨架：結構與陷阱是對的，內容標成 TODO。產生的是沒有任何檢查會抓到的部分：缺一個顯示用 annotation 會讓 UI 出現一個沒有名字的資源、沒有 set label 的 Workflow 在 UI 上看不到、沒有自己那兩個 label 的 Trigger 打開是一張空白畫布、上游改過名的欄位用舊名字照樣能通過 lint。這些都不會被 `helm lint`、CRD 驗證或 server-side dry-run 抓到。

它會先讀 chart 再寫入，所以產出的引用都指向實際存在的東西：chart 裡只有一個 SemanticLayer 就直接掛上、有好幾個就要求指名、有 SkillSet 才引用。第二個查詢工具不會重新產出第一個已經寫好的 Toolset。

### 材料，作為檔案

五份參考語料編在 binary 裡，並由 `asgard-cli init` 寫進客戶 repo。只放在某個 engagement 裡的副本會在沒人注意時過期；放在這裡的內容過期了，一次發版就替所有 engagement 修好。

```
.agents/skills/asgard-platform/
  index.md    地圖：五份都在，以及刻意沒有的東西
  aliases.md  客戶說的話 -> 該搜什麼字
  wiki/       平台有什麼，UI 上的名字對應哪個 CR
  usecase/    一種部署形狀怎麼一個欄位一個欄位組起來
  needs/      這個形狀開工前要先跟客戶拿到什麼
  brief/      這件事情前人在哪裡做錯過
  guide/      現在要做哪個決策，以及反悔的代價
```

| | 回答什麼 | 寫自哪裡 |
|---|---|---|
| `wiki/` | 平台是什麼、每一塊是給誰的、以及 UI 的名字從哪裡開始對不上 chart 宣告的資源 | 產品文件 [asgard-docs](https://github.com/asgard-ai-platform/asgard-docs)，對著 CRD 校過 |
| `usecase/` | 一種部署形狀怎麼一個欄位一個欄位組起來，以及填錯一個值的代價 | 已經在 production 跑的部署 |
| `needs/` | 這個形狀開工前一定要先從客戶那邊拿到什麼 | 訪談、各通道的憑證表，以及某個 engagement 太晚才發現的事 |
| `brief/` | 你正要做的這件事，前人在哪裡做錯過 | 真的有人做錯過的活動 |
| `guide/` | 一個決策、看起來對的那個答案，以及反悔的代價 | 三個在 production 被推翻過的決策 |

刻意沒有搜尋指令。文件就在磁碟上，用 `cat` 和 `grep` 讀：

```bash
grep -ril "allowlist" .agents/skills/asgard-platform/
cat .agents/skills/asgard-platform/wiki/processors.md
```

有兩件事 grep 不會替你做，所以先讀：

`aliases.md`，如果問題不是用英文問的。材料是英文的，客戶對話通常不是，照客戶的用詞搜會搜不到，而搜不到看起來跟「材料沒有這個主題」一樣。

`wiki/glossary.md`，確認你搜的那個詞的意思。搜到另一個意思的結果，看起來也像答案：`payment` 是 Asgard 對客戶的計費，也是客戶自己的金流閘道。

沒有 repo 時，在空目錄跑 `asgard-cli init` 就夠了，不用帳號、不碰網路，因為問題常在會議裡被問，那時還沒有目錄。

```bash
mkdir -p /tmp/asgard && cd /tmp/asgard && asgard-cli init
```

### `size`、`issue-report`

兩個都不讀 repo。

`size` 列出一個能力寫出來之前由什麼構成，這是提案最先被問的問題，也是報價的基礎。數字來自 production 的部署而不是推理，這在直覺答案錯的地方最重要：flow-agent 那幾種形狀裡完全沒有 `Agent` CR。

`issue-report` 說明這支工具的缺口怎麼回報，這也是一個 engagement 學到的東西傳到下一個的唯一途徑。缺口不要記在客戶 repo 裡：寫在某個 engagement 裡的筆記，只有那個 engagement 看得到。
`--send` 把它以 User Feedback 送到維護者的 Sentry project，只有維護者看得到，不是這個公開 repo 的 issue。它只回報工具的事，不回報客戶的事：客戶系統的問題放 Workbench（見 `workbench`）。

```bash
asgard-cli issue-report                          # 怎麼回報，以及一份報告要寫什麼
asgard-cli issue-report --new > report.md        # 證據已經填好的 body
asgard-cli issue-report --send report.md --email you@example.com   # 每個 TODO 都回答了之後送出
```

報告最後一節是「What I now know」，用來回報一個發現而不是缺陷：在部署上查到、平台會做但素材沒寫的事。

### `check`

驗收 gate 的第一步，也是唯一不需要外部工具的一步。它驗證的是 chart 渲染看不到的不變量：

```bash
asgard-cli check                    # 整個 repo
asgard-cli check erp                # project 範圍的檢查只限 erp
asgard-cli check --format json      # error 與 warning 是兩個陣列
```

- 根 README 的 project 表格跟 `projects/` 底下的目錄一致
- `.asgard-pipeline.yaml` 解析得開、沒有重複的 release 名、每個宣告的 release 都指向一個有 `Chart.yaml` 的 chart 目錄
- `assets/skills/` 底下的 runtime skill 帶著 `name` 與 `description` frontmatter，且名字與目錄相符
- `requirements/` 底下的 SDD 入口都在
- `docs/` 的 spec 層完整：必要檔案、living spec 的模組索引與磁碟上的檔案相符、日期檔名、以及 `docs/` 裡每個相對連結都解得開
- 沒有孤兒頁：`docs/` 或 `requirements/` 底下沒有任何東西連到的文件不會被讀到，而寫它的人不會發現，因為檔案還在。這是警告而非錯誤：今天記下、還沒被套用的決議，在套用之前本來就是孤兒。

### `render`、`verify`、`doctor`

有了這三個，驗收 gate 才能在 Windows 上跑。

```bash
asgard-cli render internal-dev         # manifest 到 stdout，摘要到 stderr
asgard-cli verify                      # 渲染每個 release，檢查不變量
asgard-cli verify --rendered file.yaml # 檢查一份已經渲染好的串流
asgard-cli doctor                      # 哪些外部工具在這裡，不在的話怎麼拿
```

`render` 接受的是一個 release，不是 project 加環境。一份 chart 部署到哪裡，由 `.asgard-pipeline.yaml` 裡的 Release 決定，而一份 chart 可以有好幾個。

`check` 與 `verify` 是 agent 最常用的一對，因為那是它要弄綠的 gate，所以兩個都接受 `--format json`。

舊的鏈是 `bash render.sh → yq → helm template → python3 + PyYAML`：四個外部相依，其中三個在沒有 WSL 或 Git Bash 的 Windows 上不能跑。現在的前置只有 `helm` 一個，加上一個 binary。

apiserver 自己的 CEL、pattern 與必填驗證，以及 dry run 會隱藏的 unknown-field 修剪，都需要一座叢集，而任何 client 都拿不到叢集憑證，所以它們在平台的 plan 裡跑。本機那一半是 `asgard-cli gate`，以 plan report 為準。

### helm 與 kubectl 是前置條件，不是相依

asgard-cli 的任何散佈方式都不會一起裝它們。tar.gz、zip 與 `go install` 都不帶相依 metadata。

所以由 binary 本身處理。每個需要 helm 的指令都先透過 `internal/tool` 找它，找不到就拒絕並附上這台機器的安裝指令；`asgard-cli doctor` 一次報告全部。

`init`、`project`、`request`、`task`、`question`、`decision`、`check` 都不需要這些工具。`render`、`verify` 與 `gate` 的三個 chart 步驟需要 helm。kubectl 是選用的：這個 binary 沒有任何地方跟叢集溝通，`doctor` 列出它，是因為除錯部署的人仍然想知道它在不在。

### 這些檔案

六個，每個回答不同的問題。

| 檔案 | 誰寫 | 誰讀 | 進版控 |
|---|---|---|---|
| `.asgard-pipeline.yaml` | 人 | 平台，每次 run | 是 |
| `.asgard-cli.yaml` | `asgard-cli` | 只有 `asgard-cli` | 是 |
| `.asgard-scaffold.json` | `asgard-cli init` | 只有 `asgard-cli` | 是 |
| `.agents/skills/.asgard-docs.json` | `asgard-cli skill update` | 只有 `asgard-cli` | 是 |
| `os.UserConfigDir()/asgard-cli/credentials.json` | `asgard-cli login` | `asgard-cli` | 絕不 |
| `os.UserConfigDir()/asgard-cli/profiles.json` | `asgard-cli profile set` | `asgard-cli` | 絕不（但可以直接交給同事） |

`.asgard-pipeline.yaml` 是宣告檔，也是一次部署唯一依賴的檔案：有哪些 release、每個部署哪份 chart、什麼觸發它、用哪些 key。

`.asgard-cli.yaml` 是綁定檔：這個 checkout 對到哪個 workspace、哪條 pipeline，只有這兩件事。兩欄都必填，都不推導。

這兩份紀錄各記一半，都不是版本檢查。`.asgard-scaffold.json` 記下這個 binary 出貨的每個檔案（`AGENTS.md` 與設計期 skill）是哪個版本的 CLI 寫的、寫了什麼，所以差異可以明確報成 `behind`、`edited`、`ahead` 或 `retired`。`.asgard-docs.json` 記平台那一半：抓下來的參考素材是哪個版本、抓的當下每個上游的 digest 是什麼。兩個版本號互不相干，更新它們的指令也不同。

`credentials.json` 是這支 CLI 唯一放在 repo 外的東西。權限 0600、所有 profile 共用一個檔、旁邊沒有其他檔。那裡以前還有一個 `config.json`，存預設 profile、每個 profile 的預設 workspace、以及自訂 profile 的 map；它已經移除了。它的每個欄位都是某個旗標或環境變數已經能表達的偏好，而且每一個升級時都得繼續支援。只有憑證必須放在那裡：它是機密、屬於人而不是 repo、而且推導不出來。

殘留的 `config.json` 會造成錯誤，不只是警告。退休的 `defaultProfile` 多半是 `dev`，如果靜默忽略它，每個指令都會改連 `prod`（客戶的平台）。第一個要解析 profile 的指令會拒絕執行，逐條列出退休的 key 與替代做法，並要你刪掉那個檔。

刻意不存在任何地方的東西：這個 repo 有哪些 project（讀宣告檔的 chart 路徑與 `projects/*/`）、客戶的顯示名稱（模板需要時向平台查）、以及一份 chart 原本打算長成什麼樣（沒有工具能檢查的意圖）。判準是：一個值只有在磁碟上沒有任何東西推得出它、而且平台也查不到時，才放進設定檔。

### `login`、`logout`、`whoami`

登入 Asgard 平台，讓 `pipeline` 指令能以你的身分執行。

```bash
asgard-cli login                     # 登入代管平台
asgard-cli login --profile onprem    # 登入自己設定的那一座
asgard-cli login --no-browser        # 印出 URL 而不是開瀏覽器
asgard-cli whoami                    # 問平台這個 session 是誰
asgard-cli logout --all              # 忘掉每一個 session
```

OAuth 2.0 authorization code ＋ PKCE，走 loopback redirect，這是 RFC 8252 對有瀏覽器的機器建議的做法。binary 不帶 client secret。session 存在這個使用者帳號底下，不放在客戶 repo 裡，位置是 `os.UserConfigDir()/asgard-cli/`，權限 0600。

一個 profile 就是一座 Asgard 安裝。`--profile` 逐指令指定、`ASGARD_PROFILE` 對一個 shell 生效；兩個都沒有時是 `default`，也就是代管平台。沒有任何地方記錄目前用哪個 profile。`login --set-default` 以前可以設，已經移除：一台機器上的偏好，其他人的機器上不會有。

代管平台以外的 profile 用 [`asgard-cli profile`](#profile) 寫。`ASGARD_PLATFORM_API`、`ASGARD_ISSUER`、`ASGARD_CLIENT_ID` 仍然可以逐欄位覆蓋生效的 profile，用於一次性的情況。

沒有瀏覽器的時候（CI、容器、agent sandbox）改設 `ASGARD_TOKEN`。它完全不經過儲存，不讀也不寫磁碟。

### Workbench 助手的 sandbox

平台上的 Workbench 助手會在 sandbox 裡以正在對話的成員身分跑這個 CLI。image 設了 `ASGARD_SANDBOX_MODE=true`，於是：

- 身分來自 session 檔：平台每一回合開始時寫進 `/tmp/.asgard/session.json`（0600；`ASGARD_SESSION_FILE` 可改位置），內容是成員的 access token、平台位址和這段對話所在的 workspace。沒有東西要 `login`，`login` 會直接說明。`ASGARD_TOKEN` 仍然優先；`--workspace` 與 `ASGARD_WORKSPACE` 仍排在 session 的 workspace 前面，session 的 workspace 又排在 checkout 的 binding 前面。
- 每個請求都帶 `X-Asgard-Via-Assistant: true`，讀取也一樣。
- 每一次改動都會通知畫面。 有設 `ASGARD_CLI_SIDE_EFFECT_TIMESTAMP_FILE` 時（image 設成 `/work/.asgard/side-effect-at`），只要一個呼叫在平台上改了東西，就會把 `{"at":"<RFC 3339 UTC>"}` 覆寫進那個檔，監看這個檔的 Workbench 頁面就會 refetch。算改動的包括：開 issue、留言、寫 pipeline 或 release、approve Run、出現新的 connection。讀取、被拒絕的請求，以及 git 要的 repo token 都不算。資料夾不存在時會自動建立；寫不進去只印 warning，不會讓指令失敗。檔案內容只保證有 `at`，讀的一方遇到不認得的 key 要忽略。
- git 走 workspace 的 GitHub Connection：`asgard-cli pipeline git-auth` 讓這個 CLI 成為 git 在 github.com 唯一的 credential helper，每次 fetch／push 拿一張只限那個 repo、由 GitHub App 簽的 token，不寫進磁碟。push 要 workspace 管理權限；`asgard-cli pipeline repo create` 在組織的 Connection 底下建新 repo。
- `init` 拒絕在 `/work` 本身執行，那裡並排放著所有 repo。
- 不會開本機瀏覽器，因為沒有本機。 網際網路上的頁面（GitHub 的安裝與授權頁）印成連結，給成員在自己的瀏覽器開：`pipeline connect --account <login>` 印出連結就結束，`pipeline connect --continue` 等 Connection 出現。帳號指的是 App 安裝所在的組織或使用者，不是按授權的人，要由成員自己指定：沒帶帳號、也沒有 origin remote 可以拿時，connect 不會開始，而是提醒先去問成員；之後用 `--continue --account <login>` 補上即可，沿用同一次授權；指定的帳號還沒裝 App 的話，會接著給安裝頁。只有 sandbox 連得到的頁面（`local-env` 在 127.0.0.1 的表單）用 CDP 在 sandbox 瀏覽器、成員看得到的那個分頁打開；助手用 `open_sandbox_browser` 交給成員，`local-env --wait` 每次等幾分鐘直到存檔。兩步之間表單 server 在背景跑。

### `profile`

如果你用的是代管的 Asgard 平台，不需要這一節。沒有這個檔時，每個指令都連到代管平台，這就是 `default` 的意思。這些指令用於兩種這個 binary 無法預先知道的安裝：地端部署，以及跑在本機的 stack。

```bash
asgard-cli profile list              # 設了哪些、現在生效的是哪個
asgard-cli profile show [name]       # 每個值，以及它是哪來的
asgard-cli profile set onprem --platform-api https://asgard.acme.internal \
    --issuer https://iam.acme.internal --client-id abc123
asgard-cli profile remove onprem
```

一個 profile 的每個值各自 fallback 到代管平台的值：

| | |
|---|---|
| `--platform-api` | Asgard Platform API 在哪裡 |
| `--issuer` | 替它發 token 的那座 Casdoor |
| `--client-id` | 這支 CLI 用哪個 application 出面（不是機密，每次登入都會出現在瀏覽器） |

地端安裝三個都要設。它的 API 與替它發 token 的 Casdoor 是同一座部署，一座發的 token 另一座不接受，所以只設 API 的結果是：你對著代管的 Casdoor 登入，然後把那顆 token 送到別人的伺服器。`profile show` 會印出每個值的來源，並在兩者來源不同時警告：

```
profile        onprem
platform api   https://asgard.acme.internal    profiles.json
issuer         https://iam.asgard-ai.com       the hosted platform (this profile does not set it)
client id      r21ntx0eb5igyokl3px4            the hosted platform (this profile does not set it)

WARNING: profile "onprem" takes its Platform API from profiles.json and its
identity provider from the hosted platform ...
```

它只警告不拒絕，因為本機的 Platform API 配真的 Casdoor 是合理的開發方式。

沒有 `profile use`。退休的 `config.json` 裡，「目前用哪個 profile」是唯一不會恢復的欄位：它在設定它的機器上看不見，在其他機器上不存在。選 profile 就用 `--profile`、`ASGARD_PROFILE` 或 `default`。

`profile set` 是唯一會建立 `profiles.json` 的指令，而且只在你執行它時建立，不會被其他動作順帶寫出。它不含機密，可以直接交給要設定同一座安裝的同事；憑證在另一個檔，不可以交出去。

#### 對我們自己的開發平台工作

`dev` 不是內建名字。我們的開發平台只是這支工具會遇到的其中一座安裝；把它編進去等於在每個客戶的 binary 裡放一個內部端點。

跟其他 profile 一樣設定，值請看內部的設定筆記，它們不在這個 repo 裡：

```bash
asgard-cli profile set dev \
    --issuer       <內部>  \
    --client-id    <內部>  \
    --platform-api <內部>
asgard-cli login --profile dev
export ASGARD_PROFILE=dev        # 對一個 shell 生效
```

### `workspace`、`pipeline use`

這個 checkout 對到哪個 workspace、哪條 pipeline：repo 裡推不出、平台也查不到的兩件事。

```bash
asgard-cli workspace list            # 這個帳號看得到哪些
asgard-cli workspace use <id>        # 記下 workspace
asgard-cli pipeline list             # 那個 workspace 有哪些 pipeline
asgard-cli pipeline use <id>         # 記下 pipeline
asgard-cli workspace show            # 這裡適用哪一個、為什麼是它
```

兩者都寫進 `.asgard-cli.yaml`，放在所屬的宣告檔旁邊，這個檔要進版控：clone 這個 repo 的人、以及在裡面工作的 agent，之後都不需要帶旗標。平台不讀它。

任何值都不會被推導，即使只有一個候選。沒有記錄時，指令會列出候選然後拒絕執行。只在清單只有一筆時才成立的規則，在清單變成兩筆那天會解析到沒人選過的對象，而且不會有人注意到。

不把任何 git remote 當作身分。pipeline 以前是用 checkout 的 `origin` 比對 workspace 裡的 pipeline 找出來的；一個 checkout 可以有任意多個 remote，哪一個叫 `origin` 由它的主人決定。這樣留下的缺口是：整個 repo 被複製到同一個 workspace 的另一個 repo 時，pipeline id 仍然解析得到。這個缺口寫在 `.asgard-cli.yaml` 自己的檔頭裡，不用一條會對正確輸入誤報的規則去擋。複製 repo 之後請跑 `pipeline use`。

換 workspace 會清掉 pipeline 那一行，因為一條 pipeline 屬於一個 workspace。之後每個 pipeline 指令都會拒絕執行並指出補救指令，`asgard-cli gate` 的 `binding` 步驟會紅燈，所以失敗會出現在下一個指令，而不是延後到某個具破壞性的指令。兩行要一起 commit。

解析順序，高到低：`--workspace`、`ASGARD_WORKSPACE`、checkout 的 `.asgard-cli.yaml`。只有這三個。

### `pipeline`

透過平台的 IaC pipeline 部署這個 repo。

```bash
asgard-cli pipeline connect                # 把 GitHub 接到這個 workspace
asgard-cli pipeline connections            # 已經接上的 installation
asgard-cli pipeline repos --connection X   # 其中一個看得到哪些 repo
asgard-cli pipeline create --name p        # 綁一個 repo 到新的 pipeline
asgard-cli pipeline list                   # 這個 workspace 有哪些 pipeline
asgard-cli pipeline use <id>               # 記下這個 checkout 用哪一條
asgard-cli pipeline show                   # 這個 checkout 記著的那一條
asgard-cli pipeline releases               # 它的 release，以及只被宣告的幽靈列
asgard-cli pipeline variables list --release <name>
asgard-cli pipeline runs watch --release <name> --ref <tag>
```

這些指令包裝平台 API，自己不帶任何規則。檢核在平台上跑，因為最有價值的檢查（apiserver 對每個渲染出來的 CR 做的 CEL、pattern 與必填驗證）需要一座叢集，而任何 client 都拿不到叢集憑證。所以流程是：改 chart、用 `asgard-cli gate` 檢查本機能檢查的、推上去、讀回 plan。

一次 run 有六步：`checkout → lint → variables → render_dry_run → review → apply`。`review` 由人執行。它之前的全部是 plan，plan 找到的一切都在它的報告裡。

一次推送沒有產生任何 run 時，只有 `pipeline deliveries` 會說明原因。從未建立的 run 不會留下紀錄，所以一個 tag 看起來被忽略時，要去那裡看。

### `workbench`

讀寫這個 workspace 在 Workbench 上的 issue（FDE 與客戶在平台上一起追工作的地方），並把附件歸檔進 repo。

```bash
asgard-cli workbench list --status in_progress --label blocked
asgard-cli workbench show ISS-12
asgard-cli workbench create --type question --title "<要有人回答的事>"
asgard-cli workbench update ISS-12 --status in_review --add-label data-source
asgard-cli workbench comment ISS-12 --body-file draft.md
asgard-cli workbench attach ISS-12 minutes.pdf --what "<是什麼>" --from "<角色>" --dated <YYYY-MM-DD>
asgard-cli workbench pull --pipeline <name>         # 附件拉進 references/
```

每一筆寫入都以登入者的助手身分送出：帶 `X-Asgard-Via-Assistant: true`，權限仍然是登入者本人，時間軸會標「via Asgard AI」。平台只留給本人做的操作（Pin、Lock、管理 label、刪除 issue／附件／留言），這個指令不提供，平台也會拒絕助手這樣做。

這不是 `question`、`request`、`task`，也不是 `issue-report`。 一件事放三個地方的哪一個，看的是誰要處理它：

| 誰要處理 | 放哪裡 | 指令 |
|---|---|---|
| 客戶那邊要看、要回答、要提供或要決定，或是已上線的東西壞了 | workspace 的 Workbench（客戶看得到） | `workbench create` |
| 下一個接手建置的人需要：spec、設計、還沒定的決定 | 客戶 repo 裡的紀錄 | `question add`、`request add`、`task add` |
| 這支工具或它背後平台的維護者 | 上游，只有維護者看得到的 feedback | `issue-report --new`，再 `--send` |

這支工具本身出錯，絕不是 Workbench 的 `bug`：那個 type 是客戶的部署哪裡壞了，客戶會讀到。跟客戶有關的東西一律不上上游，因為報告會離開 engagement，送進第三方的服務。在 Workbench 助手的 sandbox 裡，member 要追蹤的事放 Workbench；工具本身的缺口一樣用 `issue-report` 回報。同一張表也在 `asgard-cli workbench --help` 和 `init` 寫出的 `AGENTS.md` 裡。

`pull` 把每個附件原樣存到 `references/workbench/ISS-<n>/<attachment id>/`，先比對平台記錄的 SHA-256，再把 what、from、dated 寫進 `references/_index.md`（與 `reference add` 寫的是同一種列）。已歸檔的檔案永遠不會被覆寫。

### `audit-log`

這個 workspace 的稽核紀錄，也就是 Asgard Console › Explore 記下的內容，用你自己的 session 讀——所以誰能讀由 Console 決定（workspace owner 或平台管理員）。

```bash
asgard-cli audit-log summary --days 7    # 依事件、帳號、專案、agent 計數
asgard-cli audit-log query --days 1      # 事件本身，JSON Lines
asgard-cli audit-log dictionary          # raw key 背後的顯示名
```

每一列只有 raw key；沒有成功／失敗這個維度（事件是具名的）；資料最多延遲約 20 分鐘。

### `operate`

操作並讀回某個 release 已經部署到叢集上的 CR。上面那些指令都停在 CR 部署完成的那一刻；剩下的是它們的行為，這是 chart 表達不了、成功的 pipeline run 也不會回報的。

```bash
asgard-cli operate syncer sync <name> --release <r> --wait 10m   # 現在跑一次，並等結果
asgard-cli operate syncer executions <name> --release <r>        # 它每一次執行的結果
asgard-cli operate skill-set sync <name> --release <r>           # SkillSet 的 Syncer，經由 SkillSet 觸發
asgard-cli operate skill-set executions <name> --project <x>     # 沒有 checkout 時
asgard-cli operate trigger fire <name> --release <r> --wait 5m    # 現在觸發一次 Trigger
asgard-cli operate trigger runs <name> --release <r>              # 它的 invocation，以及 agent 自己的判斷
asgard-cli operate trigger logs <name> <invocation> --release <r>
asgard-cli operate source-set reindex <name> --release <r>        # 現在重建一次 context index
asgard-cli operate source-set index-runs <name> --release <r>     # 它的每次重建；index-logs 讀其中一次
```

Trigger 的 invocation 與 context index 的重建都是一段和 agent 的對話；agent 停下來問問題時，那次 invocation 仍然記為 succeeded。所以 `runs` 與 `index-runs` 會把 agent 自己的判斷印在狀態旁邊。

這裡只放 IaC 做不到的事。`operate` 底下沒有任何指令會修改、發佈、暫停或列出 CR：那些屬於 chart，而 `pipeline manifest` 已經能讀回 release 部署了什麼。

CR 用渲染後的 `metadata.name` 指定。`--release` 會拿這個名字對照該 release 部署的內容，並提供這些路由所需的平台 Project；`--project` 接受 id、名稱或 namespace，用在沒有 checkout 的時候。Project id 不等於 namespace，所以兩者必須擇一，而且都不會被猜。

## 發佈

發佈由 [GoReleaser](https://goreleaser.com) 驅動。推一個 tag 觸發 `.github/workflows/release.yml`：

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

一次執行產出 linux / darwin / windows × amd64 / arm64 的 binary、`.deb` / `.rpm` / `.apk` 套件與 checksum，全部掛在 GitHub Release 上。changelog 從 conventional commit（`feat:`、`fix:`）自動分組。

本機驗證而不發佈任何東西（產出落在 `dist/`）：

```bash
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```
