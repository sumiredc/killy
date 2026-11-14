# Requirements: demo-environment

## Introduction

自分の Linux サーバーを持たない審査員や来訪者が Killy を試せるようにする部分。デモ機で `killy summon` が kill しても安全なゾンビの親を作り続け、demo mode の agent がその親だけを報告して kill し、app の「デモサーバーを試す」でペアリングコードなしに公開デモのサーバーを追加する。

流れの正は [PRD: UC6](../../../docs/PRD.md#uc6-try-the-public-demo)・[UC7](../../../docs/PRD.md#uc7-run-a-demo-environment)・[Security](../../../docs/PRD.md#security)、手順の正は [ARCHITECTURE: Scanning](../../../docs/ARCHITECTURE.md#scanning)・[Kill procedure](../../../docs/ARCHITECTURE.md#kill-procedure) の 4（`not_demo`）・[Demo environment](../../../docs/ARCHITECTURE.md#demo-environment)・[Distribution](../../../docs/ARCHITECTURE.md#distribution)、画面の正は [DESIGN: Add server sheet](../../../docs/DESIGN.md#add-server-sheet)、要求と応答の形は [openapi.yaml](../../../docs/openapi.yaml) の `POST /app/v1/demo` と `demo_not_configured`。ここには確かめる基準だけを書く。

対応する issue：#22（Requirement 1）、#23（Requirement 2）、#24（Requirement 3）。

## 保証しないこと

- 本番のデモ機の用意と導入、overlay への `KILLY_DEMO_AGENT_ID` の設定（#34）
- 通常の Linux サーバー向けの `killy-agent.service` とバイナリの配布（#33）
- cgroup v2 の `0::` の行を持たない機械（cgroup v1 だけの機械）でのデモ。demo mode の agent はそこでは何も報告せず、何も kill しない
- Killy server の側での summon 由来の確認。agent だけが守る（#24 の制約）
- 公開デモの可用性。デモ機と home cluster が止まれば使えない（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed) の High availability）
- 別の PID namespace のゾンビ（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed)）
- Killy の絵（「追加できた」の 🤍 でお辞儀）。M3（#27）で付ける

## Requirements

### Requirement 1: killy summon がデモ用のゾンビの親を作り続ける

**User Story:** Killy のチームとして、デモ機にいつも kill しても安全なゾンビの親がいてほしい。自分のサーバーを持たない審査員も Killy を試せるようにするため。

#### Acceptance Criteria

1. THE summon SHALL ゾンビの親を summon とは別のプロセスとして起動し、ゾンビの子は親が作る
2. WHILE 生きている親が 3 つある THE summon SHALL 新しい親を作らない
3. WHEN 間隔（本番は 2 分）が来て、生きている親が 2 つ以下である THEN summon SHALL 新しい親を 1 つ作る
4. WHEN 親を作る THEN summon SHALL 親のコマンド名を 2 つ以上の候補（`node evil_daemon.js`・`python3 worker.py` など）から選ぶ
5. THE 親 SHALL すぐに終了する子を作り、その子を `wait()` しない。親ごとのゾンビの数と上限は PR で決め、PR 本文に書く（#22。案は design）
6. WHEN 親が終わる（kill された場合を含む） THEN summon SHALL その親をすぐに回収し、summon の子として状態 `Z` のまま残さない
7. THE `deploy/systemd/killy-summon.service` SHALL `/usr/local/bin/killy summon` を常駐させる
8. WHEN summon を `killy-summon.service` として動かす THEN 作られた親 SHALL `/proc/<pid>/cgroup` の `0::` の行が `0::/system.slice/killy-summon.service` になる
9. WHEN Linux 上で summon と `killy agent run` を動かす THEN Killy server の snapshot SHALL デモの親を 3 つまで持つ

### Requirement 2: demo mode の agent は summon が作った親だけを kill する

**User Story:** Killy のチームとして、誰でも追加して kill を送れる公開デモで、summon が作った親しか kill されないでほしい。デモ機の他のプロセスを守り、その command line も公開しないため。

#### Acceptance Criteria

1. WHEN `killy agent run --demo` で起動する THEN agent SHALL demo mode で動く。`--demo` が無ければ demo mode でない
2. THE `internal/protocol` SHALL refused の理由 `not_demo` を持つ
3. WHILE demo mode である IF kill の対象の cgroup（`/proc/<ppid>/cgroup` の `0::` の行の path）が `/system.slice/killy-summon.service` と完全に一致しない THEN agent SHALL signal を送らず `refused` / `not_demo` を返す。次のどれもこれに当たる：別の system の service、`/user.slice/user-1000.slice/user@1000.service/app.slice/killy-summon.service`（同じ名前の user unit）、`/system.slice/killy-summon.service.evil`（名前に文字を足したもの）、`0::` の行が無いもの
4. WHILE demo mode である IF 対象の cgroup のファイルが読めず、対象がまだある THEN agent SHALL signal を送らず `refused` / `not_demo` を返す
5. WHILE demo mode である IF 対象の cgroup のファイルが読めず、対象がもう無い THEN agent SHALL signal を送らず `already_gone` を返す
6. WHILE demo mode である WHEN 対象の cgroup が一致し、他の確認（agent-process-control の Requirement 2）を通る THEN agent SHALL `SIGKILL` を送り、`killed` を返せる
7. THE agent SHALL cgroup の確認を、pidfd を開いた後、signal を送る前の確かめ直しの 1 つとして行う
8. WHILE demo mode でない THE agent SHALL cgroup を確かめず、summon の cgroup に属さない親への kill も、他の確認を通れば `killed` にする
9. WHILE demo mode でない THE agent SHALL summon の cgroup に属さない親も snapshot に入れる
10. WHILE demo mode である THE agent SHALL 3 と 4 に当たる親（cgroup が一致しない親と、cgroup が読めない親）を snapshot に入れない
11. THE agent SHALL 由来の確認を Go の標準ライブラリだけで書き、プロセスの環境変数（`environ`）を読まない
12. THE `deploy/systemd/killy-agent-demo.service` SHALL `/usr/local/bin/killy agent run --demo` を root で常駐させる
13. WHEN Linux 上で `killy-agent-demo.service` として agent を動かし、summon 由来でない親に `POST /app/v1/kills` を送る THEN その親 SHALL snapshot に入らず、kill は `not_demo` を返し、親は生きたままである

### Requirement 3: app の「デモサーバーを試す」で公開デモのサーバーを追加する

**User Story:** 自分の Linux サーバーを持たない審査員として、ペアリングコードなしで公開デモのサーバーを追加し、デモのゾンビを kill したい。

#### Acceptance Criteria

1. THE Killy server SHALL 公開デモの agent の ID を、起動時に環境変数 `KILLY_DEMO_AGENT_ID` から読む
2. WHEN 正しい device token で `POST /app/v1/demo` が届き、`KILLY_DEMO_AGENT_ID` の agent が存在する THEN Killy server SHALL claim と同じ `LinkDevice`（表示名なし、hostname を使う）でその device と agent を紐付け、`200` でサーバー（`id` は agent の ID）を返す
3. WHEN 同じ device が `POST /app/v1/demo` をもう一度呼ぶ THEN Killy server SHALL 紐付けを 1 つのままにし、その device の表示名と通知の設定を変えない
4. WHEN `POST /app/v1/demo` で紐付けた THEN Killy server SHALL その device にだけ新しい `state` を送る
5. IF `KILLY_DEMO_AGENT_ID` が未設定である、または存在しない agent を指す THEN Killy server SHALL `404`（`demo_not_configured`）を返し、紐付けを作らない
6. IF device token が不正である THEN Killy server SHALL `401` を返し、紐付けを作らない
7. THE Killy server SHALL デモの親かどうかを確かめず、それを agent（Requirement 2）に任せる
8. THE Add server sheet SHALL フォームの下に「デモサーバーを試す」/「Try the demo server」を出す
9. WHEN 「デモサーバーを試す」を押して追加できた THEN app SHALL 「追加しています…」を出し、続けて「‹name› を追加しました」を出して sheet を閉じる
10. IF `POST /app/v1/demo` が `demo_not_configured` を返す THEN app SHALL 「デモサーバーは今使えません」/「The demo server is not available right now」を出す
11. IF Killy server に届かない THEN app SHALL mobile-battle-slice の Requirement 2.7 と同じ「届かない」の文言を出す。`unauthorized`・`unavailable` と JSON でない応答では、同じ Requirement の「それ以外のエラー」の 1 つの文言を出す
12. WHILE device token が無い THE app SHALL 「デモサーバーを試す」を押されても何も送らず、mobile-battle-slice の Requirement 2.3 と同じく `deviceRateLimited` か `deviceNotRegistered` を出す
13. THE 翻訳ファイル SHALL 「デモサーバーを試す」と「デモサーバーは今使えません」の文言を `ja.json` と `en.json` の両方に持つ
14. WHEN 2 台の device が同じデモの親に、ID の違う kill を順に送る、または同時に送る THEN system SHALL 片方を `killed`、もう片方を `already_gone` にし、signal を 1 回だけ送る
15. WHEN app で「デモサーバーを試す」から公開デモを追加する THEN app SHALL デモの親を battle の列に出し、長押しで kill できる
