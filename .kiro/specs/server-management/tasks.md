# Tasks: server-management

各トップレベルのタスクが 1 つの issue と 1 つの PR に当たる。テストは実装より先に書き、期待した理由で赤になることを確かめる。完了条件の正は各 issue。テストは PostgreSQL を使わず、DB は interface の向こうのメモリ上の偽物（`internal/server/memstore`）で置き換える。各 PR で `go vet ./...`・`go test -race ./...` と `cd mobile && npm run typecheck && npm run lint && npm test` を通す。

- [ ] 1. サーバーの表示名の変更・通知の停止・削除をできるようにする（#16。前提：#12、#15、#38、#43）
  - [ ] 1.1 `PATCH`・`DELETE /app/v1/servers/{id}` のテスト（その device の名前だけが変わり別の device の名前は変わらない・空と空白だけの名前で変わらない・101 文字で `400`・`notify=false` でサーバーと親が `state` に残り別の device の設定は変わらない・削除でその device の `state` から消えて別の device には残る・紐付いていない ID と UUID の形でない ID で `404` と何も変わらない・`401`・`503`・その device にだけ `state` が届く）を `memstore` で書き、赤を確かめる
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8_
  - [ ] 1.2 `server.Store` に `UpdateLink`・`UnlinkDevice` を足して `store.Store`（SQL）と `memstore` に同じ規則で入れ、claim と共有する `displayName` と handler を実装し、テストを緑にする。`memstore.SetLink` はテスト用の helper として残す。`events_test.go` の 2 か所の `SetLink` は、そのままにするか `UpdateLink` に替えるかを PR で決める
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8_
  - [ ] 1.3 `KillyClient` の `updateServer`・`removeServer` のテスト（送る path と本文・応答が無いと `unreachable`・エラーの応答で `other`・送り直さない）を書き、実装する
    - _Requirements: 1.12, 1.13, 1.14, 1.15_
  - [ ] 1.4 server sheet の画面のテスト（行を押すと開く・ホスト名の行に `state` の hostname・switch を切り替えた時点で送る・「完了」で変わったときだけ送る・削除が 1 回目では行われず 2 回目で行われる・応答が無いときとエラーのときに閉じずに赤い行・通知が off でも battle の列に残る）を書き、赤を確かめる
    - _Requirements: 1.10, 1.11, 1.12, 1.13, 1.14, 1.15, 1.16_
  - [ ] 1.5 `ServerSheet` を作り、Settings の行を押せるようにして、翻訳を足し、テストを緑にする。表示名と通知を送る時点と、失敗の表示を DESIGN の Server sheet に書き足す
    - _Requirements: 1.10, 1.11, 1.12, 1.13, 1.14, 1.15, 1.16_
  - [ ] 1.6 手元の PostgreSQL（`make db-up`）に向けて `killy server` を動かし、名前の変更・通知の停止・削除が Killy server の再起動の後も保たれていることを PR 本文に書く
    - _Requirements: 1.9_
  - [ ] 1.7 `go vet ./...`・`go test -race ./...` と、app の型検査・lint・テストを通す
    - _Requirements: 1.7_

- [ ] 2. サーバーへの接続を確認し、失敗した箇所を示す（#17。前提：#10、#16、#15、#38、#43）
  - [ ] 2.1 `internal/protocol` に `PingCommand` と check の結果の値を置き、`command.Router` の `Check` のテスト（`ok` と版・`agent_disconnected` で ping を送らない・5 秒より前とちょうど 5 秒とそれより後の `pong`・2 秒より前とちょうど 2 秒で ping を送らず実行中と直前の結果を返す・2 秒を過ぎたら送る・種類の取り違えでどちらの待ちも終わらない・別の agent の `pong`）を `clock.Fake` で書き、赤を確かめる
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.6, 2.7_
  - [ ] 2.2 `Router` の ping の待ちと 2 秒の制限、`POST /agent/v1/results` の `type: pong` を実装し、テストを緑にする
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.6, 2.7_
  - [ ] 2.3 `POST /app/v1/servers/{id}/check` のテスト（応答が `result` と版だけで時間の値を含まない・紐付いていないと `404` で ping を送らない・`401`・`503`）を書き、赤を確かめてから実装する。openapi.yaml の check に `503` を足す
    - _Requirements: 2.5, 2.8, 2.9, 2.10_
  - [ ] 2.4 agent の `CommandClient` のテスト（5 秒かかる kill の間に届いた `ping` に、kill を待たずに `pong` と版を返す・ping で `Killer` を呼ばない）を書き、赤を確かめてから実装し、`killy agent run` から版を渡す。`TestOtherEventsAreIgnored` を `ping` 以外の event で確かめるように直す
    - _Requirements: 2.11, 2.12, 2.13_
  - [ ] 2.5 `KillyClient.check` と Connection の欄のテスト（5 つの結果それぞれの文言・「確認する」から「もう一度確認」・問い合わせ中）を書き、赤を確かめてから、足りない文を翻訳ファイルに足して実装する
    - _Requirements: 2.14, 2.15_
  - [ ] 2.6 実機かエミュレータで、agent を動かしているときと止めたときに確認を行い、それぞれの表示になったことを PR 本文に書く
    - _Requirements: 2.1, 2.2, 2.15_
  - [ ] 2.7 `go vet ./...`・`go test -race ./...` と、app の型検査・lint・テストを通す
    - _Requirements: 2.4_

- [ ] 3. killy agent pair の QR を app で読み取ってサーバーを追加する（#18。前提：#6、#12、#15、#38）
  - [ ] 3.1 （新規）`pairURL` と `killy agent pair` の QR のテスト（初回と `--server` を省いた 2 回目で、QR に渡す文字列が表示したコードと保存した URL を含む `killy://pair?…` になる・`server` が percent-encode される・token を含まない）を書き、赤を確かめる
    - _Requirements: 3.1, 3.2, 3.3_
  - [ ] 3.2 `go.mod` に `github.com/skip2/go-qrcode` を足し、`RenderQR`・`TerminalQR`・`printCode` を実装して、テストを緑にする
    - _Requirements: 3.1, 3.2, 3.3_
  - [ ] 3.3 （新規）`mobile/src/killy/pair-url.ts` のテスト（受け付ける形・`killy://pair` 以外の URL・コードの無い URL・6 文字でないコード・別の `server`・末尾の `/`・host の大文字・既定の port の有り無し）を書き、赤を確かめてから実装する
    - _Requirements: 3.5, 3.6, 3.7, 3.8_
  - [ ] 3.4 `expo-camera` を入れて `app.json` に plugin を足し、Add server sheet の QR のテスト（読み取ったコードが入力欄に入り claim は送らない・別の `server` でエラーが出て要求が 1 つも送られない・形の違う URL で入力欄が変わらない・権限が無いときの案内）を書き、赤を確かめてから、ボタンと読み取りと翻訳を実装する
    - _Requirements: 3.4, 3.5, 3.6, 3.7, 3.10, 3.11_
  - [ ] 3.5 deep link のテスト（`/pair?…` で開くと 3.3 と同じ確認の後に Add server sheet が開いてコード欄にコードが入る・別の `server` では要求が送られない）を書き、赤を確かめてから `pair` の route を実装する。ARCHITECTURE と ARCHITECTURE.ja.md の Pairing の QR の項に deep link を書き足す
    - _Requirements: 3.9_
  - [ ] 3.6 development build を作り直したことと、実機で端末に出た QR を app のカメラで読み取ってサーバーを追加したことと、OS のカメラで読み取って app が開いたことを PR 本文に書く
    - _Requirements: 3.4, 3.5, 3.9_
  - [ ] 3.7 `go vet ./...`・`go test -race ./...` と、app の型検査・lint・テストを通す
    - _Requirements: 3.1, 3.5_
