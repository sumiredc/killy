# Requirements: mobile-battle-slice

## Introduction

phone の app の最小の一本道。app の土台を作り、コードでサーバーを追加して状態を受け取り、battle 画面で最も古く検出された親から長押しで kill する。これで M1 の目的（Linux 上の実際のゾンビを phone から片付けられる）を満たす。アートと動きは M3（#26・#27・#28）で付けるので、ここでは文言と最小限の表示で作る。

流れの正は [PRD: UC1](../../../docs/PRD.md#uc1-add-a-server)・[UC3](../../../docs/PRD.md#uc3-kill-zombie-parents-battle)・[UC8](../../../docs/PRD.md#uc8-app-settings)、画面の正は [DESIGN](../../../docs/DESIGN.md)、通信の正は [ARCHITECTURE: App API](../../../docs/ARCHITECTURE.md#app-api) と [Commands](../../../docs/ARCHITECTURE.md#commands)、エラーの形は relay-and-pairing の design の「Error responses」。ここには確かめる基準だけを書く。

対応する issue：#11（Requirement 1）、#12（Requirement 2）、#13（Requirement 3）。

## 保証しないこと

- キャラクターのアート、kill の演出、Killy の反応（M3）
- QR の読み取り（#18）、「デモサーバーを試す」（#24）、server sheet（#16）、push 通知（#20）、言語の切り替え画面（#21）
- サーバーを選ぶ画面と、常時の接続表示（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed)）

## Requirements

### Requirement 1: app の土台

**User Story:** 開発者として、後の画面と処理を足していける app の土台がほしい。

#### Acceptance Criteria

1. THE app SHALL Expo（TypeScript）の development build で、iOS と Android で起動する
2. THE app SHALL Battle と Settings の 2 タブを持ち、タブで切り替えられる
3. THE app SHALL 文言を日本語と英語の翻訳ファイルに持ち、`mock/app.js` の `I18N` の文言を、design の「除外するキー」を除いて両方の言語に入れる
4. THE 翻訳ファイル SHALL 片方の言語にしか無いキーを持たない
5. WHEN app が起動する THEN app SHALL 端末が日本語なら日本語、それ以外なら英語で表示する
6. THE app SHALL 接続先の Killy server の URL を build の設定から読み、その Killy server にだけ接続する
7. IF 接続先の設定が無い THEN app SHALL 起動時にエラーを出し、何も送らない
8. THE app SHALL [DESIGN: Design tokens](../../../docs/DESIGN.md#design-tokens) の色・角丸・フォントを使う

### Requirement 2: コードでサーバーを追加し、状態を受け取る

**User Story:** operator として、`killy agent pair` のコードを入力してサーバーを追加し、そのサーバーの状態を見たい。

#### Acceptance Criteria

1. WHEN app が初めて起動する THEN app SHALL push token なしで device を登録し、device token を `expo-secure-store` に保存する
2. IF device の登録が `429` で断られた THEN app SHALL その旨を出し、device token を保存せず、次の起動でやり直す
3. WHILE device token が無い THE app SHALL 「追加する」を押されても claim を送らず、登録できていない旨を出す
4. THE Add server sheet SHALL コードの入力欄で英大文字と数字だけを受け付け、6 文字になるまで「追加する」を押せなくする
5. THE Add server sheet SHALL 表示名の入力欄を持つ。空なら `name` を送らず、Killy server が hostname を使う
6. WHEN claim が成功する THEN app SHALL agent を待たずに「‹name› を追加しました」を出す
7. IF claim が失敗する THEN app SHALL `error` ごとの文言を出す：`code_unknown`・`code_used`・`code_expired`（`killy agent pair` をもう一度実行するよう案内する）、`rate_limited`（しばらく待ってからやり直すよう案内する）、Killy server に届かない、それ以外のエラー（汎用の文言）
8. WHEN `state` を受け取る THEN app SHALL 手元の状態を丸ごと置き換える
9. WHILE stream が切れている THE app SHALL 最後に受け取った状態を表示し続け、常時の接続表示は出さない
10. WHEN stream が切れる THEN app SHALL 1 秒から倍々に 30 秒まで、ランダムな揺らぎを付けて再接続する
11. THE Settings の Servers SHALL サーバーごとに表示名を出す
12. IF `state` でそのサーバーが切断である THEN Settings の Servers の行 SHALL 「最新の状態を取得できていません」を出し、バッジを出さない（[DESIGN: Settings tab](../../../docs/DESIGN.md#settings-tab) とモックの `renderServerList` の順）
13. IF サーバーが接続していてゾンビがいる THEN Settings の Servers の行 SHALL `ゾンビプロセス <n> / 親 <p>` と、ゾンビの数のバッジを出す
14. IF サーバーが接続していてゾンビがいない THEN Settings の Servers の行 SHALL 「ゾンビプロセスなし」を出し、バッジを出さない

### Requirement 3: battle 画面で長押しで kill する

**User Story:** operator として、最も古く検出された親から順に、長押しで kill したい。

#### Acceptance Criteria

1. THE battle 画面 SHALL 接続している全サーバーの親を、最初の検出時刻の古い順に 1 列に並べる。同じ時刻なら design の順で並べる
2. THE battle 画面 SHALL 切断したサーバーの親を列に入れない
3. THE battle 画面 SHALL 先頭の親のサーバー（その phone での表示名）・ゾンビの数・親の PID・コマンドラインを出す
4. WHEN 新しい `state` を受け取る THEN app SHALL 同じ規則で並べ直す。新しい親は列の最後に入り、今の親のゾンビの数は更新される
5. WHEN kill ボタンが 1 秒押され続けた THEN app SHALL 新しい kill ID（小文字の UUID v4）で `POST /app/v1/kills` を送る
6. IF kill ボタンが 1 秒より前に離された THEN app SHALL 何も送らない
7. THE kill ボタン SHALL 「長押しで kill」と `SIGKILL → PID <pid> on <server>` を出す
8. WHEN outcome が返る THEN app SHALL 次の表どおりに出し、列を扱う

   | outcome | 表示 | 列 |
   |---|---|---|
   | `killed` | kill した PID と、回収されたゾンビの数 | 次の親へ |
   | `already_gone` | すでに終了していたこと | 次の親へ |
   | `refused` | 理由（`pidfd_unavailable` を含む） | 報告され続けていれば残る |
   | `signal_failed` / `zombies_remain` | failed として、それぞれの理由 | 残る |
   | `unknown` | 結果が分からないこと。kill の成功を表す表示は出さない | 最新の状態で表示し直す |

9. IF 要求の応答が得られない THEN app SHALL 同じ kill ID で 1 回だけ送り直す
10. IF 送り直しの応答も得られない、または kill の応答が Error responses の表のエラー（`429` を除く）である THEN app SHALL ボタンの下に赤い行を出し、ボタンを押せるようにする
11. IF `429` が返る THEN app SHALL 送り直さず、ボタンの下にしばらく待つよう出し、ボタンを押せるようにする
12. WHEN 親の outcome が `killed` か `already_gone` だった THEN app SHALL その親（サーバー・pid・starttime の組）を、その親を含まない `state` が届くまで列に戻さない。別のサーバーの同じ `(pid, starttime)` は隠さない
13. WHEN `killed` か `already_gone` の直後に列が空になった THEN app SHALL All clear を出す。数は、この battle で `killed` だった親の数と、それらの親について kill を始めた時点で出ていたゾンビの数の合計で、`already_gone` は数えない。battle の区切りは design のとおり
14. IF ゾンビが無い、またはサーバーが無い THEN battle 画面 SHALL それぞれの空の状態の文言を出し、kill ボタンを押せなくする
15. WHILE kill の要求の途中である THE app SHALL Settings タブを押せなくする
16. WHILE Settings を開いている THE Battle タブ SHALL 待っているゾンビの数を出す
17. THE app SHALL Killy に言葉を話させない（[killy-character.md](../../steering/killy-character.md)）
