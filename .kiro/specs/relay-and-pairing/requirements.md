# Requirements: relay-and-pairing

## Introduction

Killy server が agent と phone の間を中継する部分。agent の登録とペアリング、snapshot の受け取り、phone の登録と紐付け、phone への状態の配信、kill の中継を作る。これで、agent-process-control の結果が phone に届き、phone の kill が agent に届く。

仕様の正は [ARCHITECTURE: Pairing](../../../docs/ARCHITECTURE.md#pairing)（Rate limits を含む）、[Killy server](../../../docs/ARCHITECTURE.md#killy-server)、[Protocol](../../../docs/ARCHITECTURE.md#protocol)、[Timing](../../../docs/ARCHITECTURE.md#timing)。ここには確かめる基準だけを書く。

対応する issue：#6（Requirement 1）、#7（Requirement 2）、#8（Requirement 3）、#9（Requirement 4）、#10（Requirement 5）。

## 保証しないこと

- QR の表示（M2、#18）、通知（M2、#19）、接続確認の ping（M2、#17）、公開デモ（M2、#24）
- 名前の変更・通知の停止・削除の API（M2、#16）
- staging と production の環境（M4）
- アカウントとログイン（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed)）

## Requirements

### Requirement 1: agent を登録し、ペアリングコードを発行する

**User Story:** operator として、サーバーで `killy agent pair` を実行するだけで、phone に入力するコードを得たい。

#### Acceptance Criteria

1. WHEN token の無い `POST /agent/v1/pairings` `{hostname}` が届く THEN Killy server SHALL agent を作り、agent token・agent の ID・コードを返す
2. WHEN agent token 付きの `POST /agent/v1/pairings` が届く THEN Killy server SHALL agent を増やさずに、同じ agent の ID と新しいコードを返す
3. IF 付けられた token が不正である THEN Killy server SHALL `401` を返し、コードを発行せず agent も作らない
4. THE Killy server SHALL コードを英大文字と数字の 6 文字にする
5. THE Killy server SHALL コードをメモリに持ち、発行から 10 分で期限切れにする
6. THE Killy server SHALL token を 32 バイトの乱数にし、DB には SHA-256 だけを保存する
7. IF 同じ接続元からの初回登録が 1 時間に 10 回に達している THEN Killy server SHALL 期間が過ぎるまで初回登録に `429` を返し、agent を作らない。数えるのは受け付けた初回登録だけで、agent token 付きの要求と `429` で断った要求は数えない
8. THE Killy server SHALL 接続元を `CF-Connecting-IP` から取り、無ければ TCP の接続元を使う
9. WHEN `killy agent pair` を初めて実行する THEN agent SHALL token と Killy server の URL を `/etc/killy/agent.json`（mode 0600）に保存し、コードと agent の ID を表示する
10. WHEN `killy agent pair` を 2 回目以降に実行する THEN agent SHALL 保存した URL に保存した token を付けて送り、コードと agent の ID を表示する
11. IF 2 回目以降の `killy agent pair` の `--server` が保存した URL と違う THEN agent SHALL 何も送らずに止め、その旨を表示する
12. THE agent SHALL 表示に agent token を含めない
13. IF `killy agent pair` が `429` を受けた THEN agent SHALL 待つよう表示し、`agent.json` を作らない
14. WHEN `killy server` が起動する THEN Killy server SHALL バイナリに埋め込んだ migration を PostgreSQL に流してから待ち受ける
15. IF migration が途中で失敗する THEN Killy server SHALL その migration を元に戻し、起動を止める
16. WHEN 流し終えた migration をもう一度流す THEN Killy server SHALL 何も変えずに成功する

### Requirement 2: agent が snapshot を報告し、Killy server が最新状態を持つ

**User Story:** operator として、サーバーのゾンビの状態が Killy server に届き続けてほしい。phone がいつ見ても最新の状態を出せるようにするため。

#### Acceptance Criteria

1. THE agent SHALL 5 秒ごとに走査する
2. WHEN 走査の結果、親の集合が前回と変わった THEN agent SHALL すぐに `POST /agent/v1/snapshots` を送る
3. WHILE 親の集合が変わらない THE agent SHALL 15 秒ごとに snapshot を送る
4. WHEN snapshot が届く THEN Killy server SHALL その agent の前の snapshot を丸ごと置き換える
5. THE Killy server SHALL 親 `(agent, pid, starttime)` ごとに最初の検出時刻を DB に持ち、同じ親では何度報告されても変えず、再起動の後も変えない
6. WHEN 親が snapshot から消える THEN Killy server SHALL その親の最初の検出時刻を消す。Killy server が止まっている間に消えた親も、再起動後のその agent の最初の snapshot に無ければ消す
7. IF snapshot の `scanned_at` が同じ agent から最後に受け付けたものより古い THEN Killy server SHALL それを捨てる
8. IF snapshot の agent token が不正である THEN Killy server SHALL `401` を返し、状態を変えない
9. THE Killy server SHALL 最後の snapshot から 60 秒たった agent と、snapshot を一度も送っていない agent を、切断とみなす
10. WHEN `killy agent run` が起動する THEN agent SHALL pidfd が使えるかを確かめる
11. IF pidfd が使えない THEN agent SHALL エラーを記録し、止まらずに報告を続ける
12. THE agent SHALL inbound のポートを開かない
13. WHEN `killy agent run` が起動する THEN agent SHALL `agent.json` から Killy server の URL と token を読む
14. IF `agent.json` が無い THEN `killy agent run` SHALL 先に `killy agent pair` を実行するよう出して止める

### Requirement 3: phone を登録し、コードで agent と紐付ける

**User Story:** operator として、phone にコードを入力するだけで、そのサーバーを追加したい。紐付けていない人には、そのサーバーを見せないため。

#### Acceptance Criteria

1. WHEN `POST /app/v1/devices` `{push_token?}` が届く THEN Killy server SHALL device を作り、device token を返す。push token は省略できる
2. IF 同じ接続元からの device の登録が 1 時間に 10 回に達している THEN Killy server SHALL 期間が過ぎるまで `429` を返し、device を作らない。数えるのは受け付けた登録だけで、`429` で断った要求は数えない
3. WHEN 有効なコードで `POST /app/v1/pairings/claim` `{code, name?}` が届く THEN Killy server SHALL agent を待たずに device と agent を紐付け、サーバー（`id` は agent の ID）を返す
4. IF `name` が無い THEN Killy server SHALL 登録時の hostname を表示名にする
5. IF コードが存在しない・使用済み・期限切れのどれかである THEN Killy server SHALL 紐付けを作らず、design の「Error responses」の表のとおり、どれかが分かるエラーを返す
6. WHEN 同じコードの claim が同時に 2 つ届く THEN Killy server SHALL 紐付けを 1 つだけ作る
7. THE Killy server SHALL 表示名とサーバーごとの通知の設定を紐付けごとに持ち、同じ agent を複数の device が紐付けられるようにする
8. IF 同じ接続元からの claim の失敗が 10 分に 10 回に達している THEN Killy server SHALL 期間が過ぎるまで、その接続元からの claim をすべて `429` にする（正しいコードでも）。`429` で断った claim は失敗として数えない
9. IF device token が不正である THEN Killy server SHALL `401` を返し、何も作らない
10. THE Killy server SHALL device token を DB に SHA-256 だけで保存する

### Requirement 4: phone に状態を SSE で配る

**User Story:** operator として、phone の画面に、紐付けたすべてのサーバーの最新のゾンビが出てほしい。

#### Acceptance Criteria

1. WHEN `GET /app/v1/events` に接続した THEN Killy server SHALL まず現在の `state` を送る
2. THE `state` SHALL その device に紐付いた全サーバー（id・その device での表示名と通知の設定・hostname・切断しているか）と、各サーバーの親（pid・starttime・cmd・ゾンビ・最初の検出時刻）を丸ごと持つ。紐付いていないサーバーの情報は含めない
3. WHEN snapshot が届く、agent が切断に変わる・接続に戻る、またはその device の紐付けが増える・変わる・消える THEN Killy server SHALL 関係する device にだけ新しい `state` を送る
4. THE Killy server SHALL 20 秒ごとに `: hb` を送る
5. THE Killy server SHALL SSE の route で書き込みの期限を外し、イベントごとに flush する
6. THE Killy server SHALL `Content-Type: text/event-stream` と `Cache-Control: no-cache` を返す
7. IF device token が不正である THEN Killy server SHALL `401` を返す

### Requirement 5: kill を agent に中継し、10 秒以内に結果を返す

**User Story:** operator として、phone で長押しした kill が agent に届き、その結果を 10 秒以内に知りたい。

#### Acceptance Criteria

1. WHEN 紐付いたサーバーへの `POST /app/v1/kills` `{id, server_id, ppid, starttime}` が届く THEN Killy server SHALL agent の命令の stream に `kill` を送り、agent の outcome をそのまま応答で返す
2. IF サーバーが紐付いていない THEN Killy server SHALL `404` を返し、agent に何も送らない
3. IF `id` が agent-process-control の Requirement 3.1 の規則（`protocol.IsUUIDv4`）に合わない THEN Killy server SHALL `400` を返し、agent に何も送らない
4. IF agent の命令の stream が無い THEN Killy server SHALL `unknown` / `agent_disconnected` を返す
5. IF 10 秒以内に結果が届かない THEN Killy server SHALL `unknown` / `timeout` を返す
6. THE Killy server SHALL 結果を、命令を送った agent からの `POST /agent/v1/results` だけから受け付ける
7. WHEN 同じ `id` の要求が同時に 2 つ届く THEN Killy server SHALL 命令を 1 回だけ送り、2 つとも同じ結果を返す
8. IF 同じ device からの kill の要求が 1 分に 30 回に達している THEN Killy server SHALL 期間が過ぎるまで `429` を返し、agent に何も送らない。認証を通った要求をすべて数え、`429` で断った要求は数えない
9. THE agent SHALL `GET /agent/v1/commands` を開き続け、切れたら 1 秒から倍々に 30 秒まで、ランダムな揺らぎを付けて再接続する
10. WHEN agent が `kill` を受け取る THEN agent SHALL agent-process-control の `Killer` に渡し、outcome を `POST /agent/v1/results` で返す
11. THE Killy server SHALL 命令の stream にも Requirement 4.4・4.5 の heartbeat と flush を使う
12. IF 命令の stream に不正な agent token で接続された THEN Killy server SHALL `401` を返す
