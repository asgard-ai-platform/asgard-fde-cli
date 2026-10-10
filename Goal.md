# Asgard FDE CLI

這個工具是為誰、做什麼。**只寫目標**——現況和還缺什麼在 `TASK.md`,指令怎麼用
在 `README.md`,主要功能怎麼實作的在 `APPROACH.md`,怎麼改在 `AGENTS.md`。

讀者是 agent:一個 FDE 帶著它做 Asgard 的客戶導入,而它手上只有客戶的
repository。那份 repository 描述客戶的系統,從不描述那些系統要跑在什麼平台上。
**這個工具是缺掉的那一半。**

## 1. 讓 Agent 能取得 Asgard AI 生態系的知識

平台的知識:Asgard 有什麼、UI 上的名字對應哪個 CR、某一種部署形狀怎麼一個
欄位一個欄位組起來、哪裡曾經被弄錯。

客戶自己系統的知識不在這裡——那個只有 engagement 賺得到,而且寫進客戶的
repository。

**離線,不需要 repo。** 問題是在會議裡問的,而一個必須先有目錄才能問的工具
不會被問。

## 2. 各種會議的幫手,能提出各種整合場景時需要的各種依賴

「依賴」指**要先從客戶那邊拿到的東西**,不是軟體套件依賴:一組憑證、一個
endpoint、一條網路路徑、一個測試環境、某個人的核准佇列。

給一個整合場景,說出那個場景要先拿到什麼,好讓沒有人在不知道代價的情況下
先答應。最貴的錯不是選錯形狀,是第三週才發現少一條網路路徑——那句話在第一週
只要一句就問得到。

會議的產出通常是一份 deck,而 deck 是這整套東西裡**唯一客戶會讀到的**,所以
它有自己的規則:哪一張截圖回答哪個問題、什麼絕不能出現在客戶的畫面上、以及
一個問題值不值得佔客戶的時間。

跟第一點的差別:第一點是「平台有什麼」,這一點是「這件事要先拿到什麼才做得
起來」。`.agents/skills/asgard-platform/needs/<場景>.md` 是這一點的落點:一個形狀一份,
說出那個場景要先拿到什麼,每一條帶著擁有那個主張的文件。

## 3. 能夠實作 IaC,charts 的部分

Asgard CR 的 Helm chart:`projects/<slug>/chart/app/`,一個 project 一份 chart。
部署到哪裡不在 chart 裡:repo 根目錄一份 `.asgard-pipeline.yaml` 宣告有哪些
Release,一個 Release 綁一份 chart 到一個 Platform Project(= 一個 namespace),
由一條 tag 或 branch 規則觸發。

**namespace 與 environment id 不在範圍內,而且是刻意的。** 它們由 Platform 在每次
Run 注入成 `.Values.asgard.*`,chart 讀就好 —— 這比它們以前的樣子(要等
namespace 建好、去把 id 抄進一個 per-env 的 values 檔)少了一整條會出錯的順序。

**能不能部署也不在範圍內。** Platform 會用真值渲染、把每個 CR 送進 apiserver
做 dry run,那是本機做不到的:client 從頭到尾不會拿到叢集憑證。這個工具做的是
另外那一半 —— 過得了 dry run、runtime 才炸的那一類檢查,以及把 Plan 的結果讀回來。

## 4. 查不到需要的知識時,Agent 從 asgard-cli 的輸出就知道去哪裡回報

回報送到維護者的 Sentry User Feedback,不是公開 repo 的 issue,因為報告是在
engagement 進行中寫的。

`asgard-cli issue-report` 印出怎麼寫、怎麼送;`--help` 說一份報告要寫什麼;
`--new` 直接產出已經填好證據的 body;`--send` 把它送出去。

**要從工具自己的輸出講出來**,不能要 agent 自己想到有這條路。這裡的「prompt」
指工具印出來的指引,不是 LLM 的 system prompt。

為什麼一定要有:語料庫是編進 binary 的——這是對的,一份過期文件改一次所有
engagement 都受惠——代價是 engagement 學到的東西**沒有別的地方寫回去**。

---

## 四點的關係

**第一點是脊椎。** 二和三是同一份知識的兩個出口:在會議上講出來,和寫進
chart。四是知識不夠時的回填路徑。

所以**知識庫是產品,不是指令表面**,而讓 agent 搜得到它是全部的工程問題。
那跟讓人讀得懂是不同的要求:agent 靠指標和字詞命中找文件,不會瀏覽索引,
也分不出拿到的答案是不是它問的那個詞的另一個意思。

推論:**任何「描述另一份文件」的東西都不能用手維護。** 一列不再說出那份文件裡
有什麼,讀起來跟一列正確的完全一樣 —— 而且沒有人會發現,因為它看起來有答案。
凡是推導得出來的就推導,推導不出來的(那份文件屬於哪一組、要為了什麼來看它)
寫在**那份文件自己身上**,索引從它生成。

形式是 [llm-wiki](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f),
理由只有一個:**綜合只做一次、寫進文件**,不是每次查詢重做一遍。

Google 的 [Open Knowledge
Format](https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing)
從另一邊走到同一個形狀:markdown 檔案、比目錄樹更豐富的連結圖、不需要重建索引。
但**它明說它只解格式**——過期、改一處要連帶改哪些、連過去的東西還在不在,規格都
沒有機制。那三件事才是這個工具真正在做的:一份可攜但已經不對的知識庫,比沒有更
危險,因為拿到它的 agent 不會回報失敗,它會把沉默填滿。

**而且那個缺口不是 OKF 一家的。** [WikiSkill](https://arxiv.org/abs/2608.27454)
把 agent 自己的執行經驗編成一份 wiki,主題完全不同(它寫的是 agent 的行為,我們
寫的是外部平台),卻走到同一個三層形狀,然後在 Limitations 裡寫下同一句話:它沒有
自動修剪 wiki 的機制,也沒有任何東西驗證一條記錄還成不成立。

兩份獨立的設計、兩種完全不同的題材,都把「過期」留在規格外面。**那不是巧合:格式
設計得出來,過期設計不出來** —— 它只能靠記下「這一條是對著哪個版本讀的」,然後有
東西去比對那個版本有沒有動。每條出處標記上記下的 commit 和
`go run ./hack sources` 就是那件事,而它們是這個工具唯一沒有前例可抄的部分。
