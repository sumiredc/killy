# Tasks: web-demo

各トップレベルのタスクが 1 つの issue と 1 つの PR に当たる。テストは実装より先に書き、期待した理由で赤になることを確かめる。完了条件の正は各 issue。テストは PostgreSQL を使わない。各 PR で、Go を変えるなら `go vet ./...` と `go test -race ./...`、app を変えるなら `cd mobile && npm run typecheck && npm run lint && npm test` も通す。

- [ ] 1. Expo の Web 版が動くかを確かめる（#129。前提：#128）
  - [ ] 1.1 `mobile/` に Web のビルドの依存を足し、`npx expo export --platform web` を通す。native のテスト（`npm test`）を変更前に実行して結果を取り、変更後も同じであることを確かめる
    - _Requirements: 1.1, 1.4_
  - [ ] 1.2 手元の Killy server（CORS を許可）と擬似 agent で、Web のビルドをブラウザで開き、絵と動き・SSE・1 秒の長押し（マウスとタッチ）・足りない依存と警告を確かめ、結果と直し方を #129 に書く
    - _Requirements: 1.2, 1.3_

- [ ] 2. server：CORS を許可するオリジンを環境変数で受け取り、staging だけで許可する（#130。前提：#128）
  - [ ] 2.1 `ParseCORSOrigins` と、CORS の層（`Server.CORSOrigins`）の最小のスタブを置く
    - _Requirements: 2.1_
  - [ ] 2.2 `ParseCORSOrigins` の表のテスト（正しい値・空・空白と末尾の `/`・`*`・パス・クエリ・`http`/`https` でないスキーム・空の項目）と、CORS の層のテスト（許可したオリジンの通常の応答・エラーの応答・SSE・preflight、許可していないオリジン・`Origin` なし・未設定、`/agent/v1/`、`Allow-Credentials` が無いこと、token の無い要求が `401` のまま）を書き、赤を確かめる
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9_
  - [ ] 2.3 `ParseCORSOrigins` と CORS の層を実装し、`Handler()` に差し、`cmd/killy/server.go` で `KILLY_CORS_ORIGINS` を読む。テストを緑にする
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9_
  - [ ] 2.4 `*` を返す・完全一致を前方一致に変える・`/agent/v1/` にも付ける・preflight が認証に進む、のそれぞれにすると対応するテストが落ちることを確かめ、PR 本文に書く
    - _Requirements: 2.4, 2.5, 2.6, 2.8_
  - [ ] 2.5 `deploy/overlays/stg/` に `KILLY_CORS_ORIGINS` を足す（prd には足さない）。`kustomize build` の出力で確かめる
    - _Requirements: 2.10_
  - [ ] 2.6 `docs/ARCHITECTURE.md` と `docs/ARCHITECTURE.ja.md` に CORS を書き、`docs/openapi.yaml` に preflight を書くか判断する
    - _Requirements: 2.11_
  - [ ] 2.7 手元の PostgreSQL（`make db-up`）に向けて `KILLY_CORS_ORIGINS` つきで `killy server` を動かし、`curl` で preflight と SSE の応答のヘッダを確かめ、PR 本文に書く。staging の image を審査の前に更新する期限を PR 本文に書く
    - _Requirements: 2.4, 2.5, 2.12_

- [ ] 3. app：Web では device token と設定を localStorage に保存し、通知とカメラを出さない（#131。前提：#128、#129）
  - [ ] 3.1 `tokenKey` を `token-key.ts` に移し（`deps.ts` からも同じ名前で読める）、native のテストが変わらず緑であることを確かめる
    - _Requirements: 3.7_
  - [ ] 3.2 `deps.web.ts`・`prefs.web.ts`・`notifications.web.ts` のテスト（`localStorage` の偽物での保存と読み出し・値が正しくないときの扱い・`localStorage` が無い、読み書きで例外になるときの扱い・通知の許可が `denied` で push token が取れないこと・native の依存を import していないこと）を書き、最小のスタブで赤を確かめる
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.8_
  - [ ] 3.3 3 つの Web 用の実装を書き、テストを緑にする
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.8_
  - [ ] 3.4 Settings の通知の行と `AddServerSheet` の QR のボタンが Web で出ないことの画面のテスト（`Platform.OS` を `web` にして）を書き、赤を確かめてから実装する。native（`ios`）では今までどおり出ることも確かめる
    - _Requirements: 3.5, 3.6, 3.7_
  - [ ] 3.5 `npm run typecheck && npm run lint && npm test` と `npx expo export --platform web` を通し、#129 で出た直しを入れる
    - _Requirements: 3.7, 3.8_

- [ ] 4. app：Web 版をデモ専用にし、開くとデモサーバーの battle 画面を出す（#132。前提：#131）
  - [ ] 4.1 `webDemoEnabled` のテスト（`1`・`0`・空・未設定・空白つきの `1`）と `claimMessages` を `claim-messages.ts` に移すこと（native のテストは緑のまま）
    - _Requirements: 4.1_
  - [ ] 4.2 `WebDemoProvider` の画面のテスト（登録が済むと `POST /app/v1/demo` が 1 回・battle にデモの親が出る・登録が `429`・届かないときに呼ばず理由が出る・デモの追加の失敗 3 種の文言・無効なときと `Platform.OS` が `ios` のときは呼ばない・Settings に Add server が出ない・保存された token があれば登録しない）を書き、最小のスタブで赤を確かめる
    - _Requirements: 4.2, 4.3, 4.4, 4.5, 4.6_
  - [ ] 4.3 `WebDemoProvider`・battle 画面の理由の表示・Settings の出し分けを実装し、テストを緑にする
    - _Requirements: 4.2, 4.3, 4.4, 4.5, 4.6, 4.7_
  - [ ] 4.4 PC の幅の枠のテスト（Web で `maxWidth` が 430、native では枠が無い）を書き、赤を確かめてから実装する
    - _Requirements: 4.9_
  - [ ] 4.5 手元の Killy server と擬似 agent で、新しいブラウザ（`localStorage` が空）で開いてデモのゾンビが出ること、1 秒の長押しで kill が送られること、1 秒より前に離すと送られないこと、再読み込みで登録し直さないこと、PC の幅の枠を確かめ、PR 本文に書く
    - _Requirements: 4.7, 4.8, 4.9_
  - [ ] 4.6 `npm run typecheck && npm run lint && npm test` を通す
    - _Requirements: 4.3_

- [ ] 5. site：Web 版をビルドして /demo/ に配置する（#133。前提：#132、#130）
  - [ ] 5.1 `app.json` に `baseUrl`・`web.output` を足し、`EXPO_PUBLIC_KILLY_SERVER_URL=https://stg-killy.sumire.pink EXPO_PUBLIC_KILLY_WEB_DEMO=1 npx expo export --platform web` の出力を、ローカルの静的な server の `/demo/` で開いて確かめる。出力が外の URL を読まないことを `grep` で確かめる
    - _Requirements: 5.1, 5.2_
  - [ ] 5.2 `deploy-site.yml` に Node のセットアップとビルドと `out/demo/` への配置、`paths` の `mobile/**`、確認の `/demo/` を足す。ビルドの失敗でアップロードされないことを、ビルドを壊して手元で確かめる
    - _Requirements: 5.3, 5.4, 5.5, 5.7_
  - [ ] 5.3 `/demo/settings` を直接開いたときの扱いを確かめ、`.htaccess` を書き換えずに済む形にするか、その制限を PR 本文に書く
    - _Requirements: 5.6_
  - [ ] 5.4 マージ後に `https://killy.sumire-sakamoto.co.jp/demo/` で #132 の動きになることを、人が確かめる手順を PR 本文に書く
    - _Requirements: 5.8_

- [ ] 6. docs：Web 版のデモを README・PRIVACY・ARCHITECTURE・PRD に書く（#134。前提：#132、#133、#130）
  - [ ] 6.1 `README.md`・`docs/PRIVACY.md`・`docs/PRIVACY.ja.md`・`docs/ARCHITECTURE.md`・`docs/ARCHITECTURE.ja.md`・`docs/PRD.md` を直し、`go run ./tools/privacypage` で `site/privacy/` を作り直す
    - _Requirements: 6.1, 6.2, 6.3, 6.4_
  - [ ] 6.2 `go test ./...` を通し、「Try the demo server」「judges' builds」「TestFlight」を `grep` して食い違いが無いことを確かめる。紹介サイトに導線を足すかの提案を PR 本文に書く
    - _Requirements: 6.2, 6.5, 6.6_
