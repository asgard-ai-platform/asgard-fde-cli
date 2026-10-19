# assets/knowledge — 部署後的 agent 拿來回答問題的知識頁面

一個知識 Drive 一個目錄,裡面是整理好的頁面,每個資料夾一份 `index.md`。

## 這裡是什麼

部署後的 Asgard agent 要拿來回答問題的文件:產品說明、FAQ、客服話術、政策。
它們隨這個 repo 被 git Syncer 同步進一個 SourceSet(Drive)的 volume,agent 用
`sourceSetMounts` 掛載其中一個目錄來讀。

這個目錄一開始是空的,這是刻意的。路徑先定下來,每個 engagement 的 `subPath`
才會一樣,接手的 agent 也知道要去哪裡找。

```
assets/knowledge/
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
- 不是給在這個 repo 工作的 coding agent 讀的。那些在 `.agents/skills/` 和 `docs/`。
- 不是資料庫查得到的東西。數量、價格、庫存、聯絡方式要用查詢工具,放進頁面會讓
  agent 轉述一個應該直接讀出來的數字。
- 不放密鑰,也不放個資。這裡的東西會原封不動進到 volume。

## 怎麼生效

```
commit → git Syncer 把這個 repo clone 到 volume 的 git/ 底下
       → SourceSet 不設 contextIndex
       → SandboxBlueprint.sourceSetMounts 掛 subPath git/assets/knowledge/<drive>/
       → agent 的 prompt 叫它從 index.md 開始讀
```

```yaml
  sourceSetMounts:
    value: '[{"sourceSetName": "ss-<name>-knowledge", "subPath": "git/assets/knowledge/<drive>/", "mountPath": "/knowledge", "readOnly": true}]'
```

Syncer、SourceSet 怎麼寫,什麼時候要改成建 Context Index,見
`.agents/skills/asgard-platform/usecase/knowledge-drive.md` 的
"When unsure: a written wiki, with no index"。頁面怎麼維護見 `.agents/skills/llm-wiki/`。

改動這個目錄的名字時,chart 裡的 `subPath` 要一起改。
