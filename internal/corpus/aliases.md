# The index: what a customer says, and what to search for

This is an index, not a page about the platform. It lives beside `wiki/`
rather than in it, next to `index.md`, because both are the wiki's own
bookkeeping. Inside the corpus it competed with the pages it points at: it lists
every alias, so it was often the one document carrying every term of a
translated query, and a search for 電商 returned this table instead of
`wiki/taiwan-channels.md`.

Apply both tables below to a query before searching. A term with a row here
does not match the material as the customer said it.

## How a row gets here

Every row is a term somebody searched for. Add one when a search came back
empty and the subject turned out to exist under another name; that is the only
test, and a row nobody has needed is a guess. File it with
`asgard-cli issue-report --new`, then `--send`, so every engagement gets it.

Point a row at a page where one page answers it: the pointer is checked by
`asgard-cli audit-material --links`, which a bare word would not be.

Every term in the right-hand column is one the material uses. Grep this
directory for a term before adding it: a row sending a reader to a word the
material does not use gives an empty search, and an empty search looks like a
subject nobody covered.

Use the term a page actually writes. A near-miss is worse than a dead term:
`audit` on its own lands mostly on this tool's own `audit-material`, so the row
says `audit event`, which is what the page says.

## What a customer says, in the words this material uses

The corpus is English and a customer conversation is not, so a query taken from
what somebody said finds nothing, and the reader concludes the subject is
missing when it is only named differently.

These replace the word. A Chinese term appears nowhere in an English corpus,
so keeping it in the query only adds a term that matches nothing.

| they said | search for |
|---|---|
| 原始碼 | source, repository |
| 程式碼 | source |
| 文件 | documentation, sources |
| 電商 | commerce, marketplace, channel - and `wiki/taiwan-channels.md` |
| 庫存 | inventory, stock |
| 訂單 | order |
| 客服 | support, customer service, help desk - and `usecase/chat-channel.md` |
| 權限 | permission, scope, console - and `wiki/console.md` |
| 稽核 | audit-log, audit event - and `wiki/console.md` |
| 入口 | entry point, channel - and `needs/chat-channel.md` |
| 上限, 限制 | limit, quota - and `wiki/integration.md` |
| 步驟 | step - and `wiki/glossary.md` |
| 報表 | dashboard, report, view - and `usecase/mimir-dashboard.md` |
| 儀表板 | dashboard - and `usecase/mimir-dashboard.md` |
| 知識庫 | knowledge, drive, context index |
| 白名單 | allowlist, outbound - and `wiki/operations.md` |
| 網路 | network, reachable, allowlist |
| 排程 | schedule, trigger, cron |
| 沒反應 | returns nothing, never runs - and `wiki/green-and-doing-nothing.md` |
| 第一則訊息沒回應 | first message, listen-message, terminal - and `usecase/flow-agent-supervisor.md` |
| 沒作用 | returns nothing, never runs - and `wiki/green-and-doing-nothing.md` |
| 空的 | returns nothing - and `wiki/green-and-doing-nothing.md` |
| 核准 | approval, consent, requestConsent - and `usecase/write-path.md` |
| 寫入 | write - and `usecase/write-path.md` |
| 投影片 | deck, slides - and the proposal-deck design-time skill |
| 簡報 | deck, slides - and the proposal-deck design-time skill |
| 截圖 | screenshot - and `wiki/screenshots.md` |
| 瀏覽器 | browser, CDP - and the desktop-browser design-time skill (on a desktop; the Workbench sandbox has its own) |
| 後台 | admin console - and the desktop-browser design-time skill |
| 語意層 | semantic layer |
| 資料庫 | database, DataConnector |
| 技能 | skill, SkillSet |
| 價格 | cost, billing - and `wiki/fehu.md` |
| 計費 | billing - and `wiki/fehu.md` |

## Names the material covers

Somebody searched the reference deployments for each of these and wrote down what
came back, so the row routes to a page that has the answer.

These are added to a query rather than replacing it, unlike the table above:
the name may be written verbatim in a page (SHOPLINE is), and that page is the
best answer. Replacing the name would lose it.

| they said | also search for |
|---|---|
| shopline | commerce, marketplace, channel - and `wiki/taiwan-channels.md` |
| shopee | commerce, marketplace, channel - and `wiki/taiwan-channels.md` |
| 蝦皮 | commerce, marketplace, channel - and `wiki/taiwan-channels.md` |
| momo | commerce, marketplace, channel - and `wiki/taiwan-channels.md` |
| pchome | commerce, marketplace, channel - and `wiki/taiwan-channels.md` |
| coupang | commerce, marketplace, channel - and `wiki/taiwan-channels.md` |

## Names it only routes

The material does not name these. The row exists because somebody searched
for it and the search was recorded; it routes to the shape the thing belongs to,
which is what this material has.

A row that routes looks the same as a row that answers, so a reader can take
results about a shape as results about a product. Move a row up to
`## Names the material covers` only when somebody has done the search and
recorded the answer.

| they said | also search for |
|---|---|
| 綠界 | payment gateway, requestConsent, approval, external api |
| ecpay | payment gateway, requestConsent, approval, external api |
| 藍新 | payment gateway, requestConsent, approval, external api |
| newebpay | payment gateway, requestConsent, approval, external api |
| 第三方支付 | payment gateway, requestConsent, approval, external api |
