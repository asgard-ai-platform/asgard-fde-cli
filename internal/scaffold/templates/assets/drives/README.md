# assets/drives — 每個知識 Drive 的頁面,給部署後的 agent 拿來回答問題

一個知識 Drive 一個目錄,裡面是整理好的頁面,每個資料夾一份 `index.md`。

## 這裡是什麼

部署後的 Asgard agent 要拿來回答問題的文件:產品說明、FAQ、客服話術、政策。
它們隨這個 repo 被 git Syncer 同步進一個 SourceSet(Drive)的 volume,agent 用
`sourceSetMounts` 掛載其中一個目錄來讀。

這個目錄一開始是空的,這是刻意的。路徑先定下來,每個 engagement 的 `subPath`
才會一樣,接手的 agent 也知道要去哪裡找。

```
assets/drives/
  <drive>/                 一個 Drive 一個目錄,例如 customer-service/
    index.md               這個目錄有哪些頁面、每一頁回答什麼問題
    <topic>.md
    <subfolder>/
      index.md
      <topic>.md
```

`index.md` 每一列寫「這一頁回答什麼問題」,不是只寫標題。agent 靠這一列決定要不要
打開那一頁,描述寫得含糊,agent 就會打開錯的頁面,或每一頁都打開。

頁面之間用相對路徑連結。同一個 Drive 的頁面彼此連得到;連到別的 `<drive>/` 的頁面,
掛載後不一定存在。

## 這裡不是什麼

- 不是客戶交來的原始檔案。原始檔案用 `asgard-cli reference add` 歸檔到
  `references/`,這裡放的是從原始檔案整理出來的頁面。整理時在頁面上寫出處:
  從哪份文件、哪一天、有沒有對過真的系統。
- 不是 runtime 技能。agent 要套用的規則、口徑、操作步驟放 `assets/skills/`;
  agent 要拿來查、拿來引用的文件放這裡。
- 不是整理跟客戶對談的地方。會議記錄、決議、系統現在的行為寫在 `docs/`,那是給 FDE
  和 coding agent 讀的 engagement 筆記,裡面有內部討論和還沒確認的推測,不能讓部署後的
  agent 讀到。這裡放的是從那些內容整理出來、可以直接給終端使用者看的頁面。
- 不是給在這個 repo 工作的 coding agent 讀的。那些在 `.agents/skills/` 和 `docs/`。
- 不是資料庫查得到的東西。數量、價格、庫存、聯絡方式要用查詢工具,放進頁面會讓
  agent 轉述一個應該直接讀出來的數字。
- 不放密鑰,也不放個資。這裡的東西會原封不動進到 volume。員工的假勤、客戶的訂單這類
  「某個人的資料」要用查詢工具問來源系統,不能寫成頁面。
- 不放完全不能上平台的內容。Drive 的 git Syncer 用 `includePaths` 只同步
  `assets/drives/<drive>/`,但這個 repo 的 SkillSet 也會同步它,沒設 `includePaths`
  的 Syncer 會把整個 repo 帶上平台。這類內容放另一個 repo,或客戶自己的 Google Drive
  資料夾,接獨立的 SourceSet。
- 只給某一個 agent 看的內容(薪資級距、考績規則)放自己的 `<drive>/`,用自己的
  SourceSet 和 Syncer,`includePaths` 只列那個資料夾。不要和別的 Drive 共用一個
  SourceSet 再靠 `subPath` 分開:同一個 volume 上的東西,掛載設定一改就看得到。

## 怎麼生效

```
commit → git Syncer 只把 assets/drives/<drive>/ clone 到 volume 的 git/ 底下
         (includePaths,保留 repo 裡的路徑)
       → SourceSet 不設 contextIndex
       → SandboxBlueprint.sourceSetMounts 掛 subPath git/assets/drives/<drive>/
       → agent 的 prompt 叫它從 index.md 開始讀
```

```yaml
  sourceSetMounts:
    value: '[{"sourceSetName": "ss-<drive>", "subPath": "git/assets/drives/<drive>/", "mountPath": "/knowledge", "readOnly": true}]'
```

`asgard-cli add knowledgedrive <drive> --repo <這個 repo 的 git url>` 寫出這個
SourceSet 和 Syncer。`includePaths` 列的資料夾在同步的那個 revision 上必須存在,
不然 Syncer 會失敗、Drive 維持原樣,所以打 release tag 之前先 commit
`assets/drives/<drive>/index.md`。

Syncer、SourceSet 怎麼寫,什麼時候要改成建 Context Index,見
`.agents/skills/asgard-platform/usecase/knowledge-drive.md` 的
"When unsure: a written wiki, with no index"。頁面怎麼維護見 `.agents/skills/llm-wiki/`。

改動這個目錄的名字時,chart 裡的 `subPath` 要一起改。
