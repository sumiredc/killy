# Tasks: relay-and-pairing

各トップレベルのタスクが 1 つの issue と 1 つの PR に当たる。テストは実装より先に書き、期待した理由で赤になることを確かめる。完了条件の正は各 issue。テストは PostgreSQL を使わず、DB は interface の向こうのメモリ上の偽物（`internal/server/memstore`）で置き換える。

- [x] 1. agent を登録し、ペアリングコードを発行する（#6。前提：agent-process-control の 1）
  - [x] 1.1 `killy server` の起動、`Store`（pgx の pool）、goose の migration（agents）を作る。migration の後に待ち受け、失敗したら止まることのテストを書く。migration を 2 回流して何も変わらないことは手元の PostgreSQL で確かめる
    - _Requirements: 1.14, 1.15, 1.16_
  - [x] 1.2 `RateLimiter` と `ClientAddr` のテストを書き、実装する
    - _Requirements: 1.7, 1.8_
  - [x] 1.3 `POST /agent/v1/pairings` と `PairingCodes` のテスト（初回・token 付き・不正な token・文字種・期限の前とちょうどと後・`429`・token の保存の形）を書き、赤を確かめる
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7_
  - [x] 1.4 handler と `PairingCodes` を実装し、テストを緑にする
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7_
  - [x] 1.5 `killy agent pair`（`agent.json` の中身と mode、2 回目は保存した URL に送る、`--server` の食い違い、表示、`429`）のテストを書き、実装する
    - _Requirements: 1.9, 1.10, 1.11, 1.12, 1.13_
  - [x] 1.6 `go vet ./...` と `go test ./...` を通し、手元で `killy server` と `killy agent pair` を 2 回動かして agent が 1 つのままであることを PR 本文に書く
    - _Requirements: 1.1, 1.2_

- [x] 2. agent が snapshot を報告し、Killy server が最新状態を持つ（#7。前提：#4、#6）
  - [x] 2.1 migration（first_detections）を作り、`LiveState` のテスト（置き換え・検出時刻・再起動・消えた親・再起動中に消えた親・古い `scanned_at`・不正な token・切断の判定の前とちょうどと後・一度も送っていない agent）を書き、赤を確かめる
    - _Requirements: 2.4, 2.5, 2.6, 2.7, 2.8, 2.9_
  - [x] 2.2 `POST /agent/v1/snapshots` と `LiveState` を実装し、テストを緑にする
    - _Requirements: 2.4, 2.5, 2.6, 2.7, 2.8, 2.9_
  - [x] 2.3 `Reporter` のテスト（5 秒ごとの走査・変わったらすぐ・15 秒ごと）、起動時の pidfd の確認のテスト（使えないときに記録して続ける）、`agent.json` の読み込みと無いときのテストを書き、赤を確かめる
    - _Requirements: 2.1, 2.2, 2.3, 2.10, 2.11, 2.13, 2.14_
  - [x] 2.4 `killy agent run` と `Reporter` を実装し、テストを緑にする
    - _Requirements: 2.1, 2.2, 2.3, 2.10, 2.11, 2.13, 2.14_
  - [x] 2.5 `go vet ./...` と `go test ./...` を通し、Linux 上で実際のゾンビが届くことと、agent が LISTEN していないことを PR 本文に書く
    - _Requirements: 2.2, 2.12_

- [x] 3. phone を登録し、コードで agent と紐付ける（#8。前提：#6）
  - [x] 3.1 migration（devices・links）と、device の登録のテスト（push token なし・回数制限・token の保存の形・不正な token）を書き、赤を確かめる
    - _Requirements: 3.1, 3.2, 3.9, 3.10_
  - [x] 3.2 claim のテスト（agent を待たない・hostname・3 種類のエラーの status と `error`・同時の claim・2 台の device と別々の表示名・claim の失敗の制限）を書き、赤を確かめる
    - _Requirements: 3.3, 3.4, 3.5, 3.6, 3.7, 3.8_
  - [x] 3.3 `POST /app/v1/devices` と `POST /app/v1/pairings/claim` を実装し、`go vet ./...` と `go test -race ./...` を通す
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9, 3.10_

- [x] 4. phone に状態を SSE で配る（#9。前提：#7、#8）
  - [x] 4.1 共通の `sse.Writer`（heartbeat・書き込み期限の解除・flush・ヘッダ）のテストを書き、実装する
    - _Requirements: 4.4, 4.5, 4.6_
  - [x] 4.2 `GET /app/v1/events` と `Broker` のテスト（最初の `state`・紐付いた device だけ・表示名と通知の設定・id・切断と接続・紐付けの増減・`401`）を書き、赤を確かめる
    - _Requirements: 4.1, 4.2, 4.3, 4.7_
  - [x] 4.3 `Broker` を実装し、`LiveState` と claim から呼ぶ
    - _Requirements: 4.1, 4.2, 4.3_
  - [x] 4.4 `go vet ./...` と `go test -race ./...` を通し、手元で `curl` の stream を 10 分以上開き続けて切れなかったことを PR 本文に書く
    - _Requirements: 4.4_

- [x] 5. kill を agent に中継し、10 秒以内に結果を返す（#10。前提：#4、#5、#9）
  - [x] 5.1 `CommandRouter` と `POST /app/v1/kills`・`POST /agent/v1/results` のテスト（中継・紐付いていない・`400`・切断・期限の前とちょうどと後・別の agent の結果・同じ ID の同時の要求・kill の回数制限）を書き、赤を確かめる
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8_
  - [x] 5.2 命令の stream のテスト（heartbeat・`401`）を書き、赤を確かめる
    - _Requirements: 5.11, 5.12_
  - [x] 5.3 `CommandRouter` と handler、命令の stream を実装し、テストを緑にする
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 5.11, 5.12_
  - [x] 5.4 agent の `CommandClient` と `Backoff` のテスト（基準値と揺らぎ・`Killer` に渡して結果を送る）を書き、実装する
    - _Requirements: 5.9, 5.10_
  - [x] 5.5 Killy server と agent をつないだテストで、同じ kill ID の 2 回で signal が 1 回であることを確かめる
    - _Requirements: 5.1, 5.10_
  - [x] 5.6 `go vet ./...` と `go test -race ./...` を通し、Linux 上の実際のゾンビの親を `curl` で kill して `killed` が返ることを PR 本文に書く
    - _Requirements: 5.1_
