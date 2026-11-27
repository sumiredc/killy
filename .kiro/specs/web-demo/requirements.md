# Requirements: web-demo

## Introduction

審査員がインストールなしで Killy を試せるように、今の Expo アプリ（`mobile/`）を Web 版としてビルドし、紹介サイトの `https://killy.sumire-sakamoto.co.jp/demo/` に置く。Web 版は staging の Killy server（`https://stg-killy.sumire.pink`）につながり、開くと公開デモのサーバーを追加して battle 画面を出す。ブラウザから別のオリジンの server を呼ぶので、staging の Killy server に CORS を足す。

目的は PRD の [UC6](../../../docs/PRD.md#uc6-try-the-public-demo)（自分のサーバーを持たない審査員が Killy を試す）を、アプリを入れない形でも満たすこと。流れの正は UC6、デモの安全（summon 由来の親だけを kill する）は [PRD: Security](../../../docs/PRD.md#security) と [ARCHITECTURE: Kill procedure](../../../docs/ARCHITECTURE.md#kill-procedure) の 4（`not_demo`）、配置の正は [ARCHITECTURE: Distribution](../../../docs/ARCHITECTURE.md#distribution)。ここには確かめる基準だけを書く。

対応する issue：#128（この spec）、#129（Requirement 1）、#130（Requirement 2）、#131（Requirement 3）、#132（Requirement 4）、#133（Requirement 5）、#134（Requirement 6）。

## 保証しないこと

- Web 版での push 通知、ペアリングのコードの入力、QR の読み取り（Web 版はデモ専用）
- production（`https://killy.sumire.pink`）に接続する Web 版。production の server に CORS もデモも足さない
- Web 版の可用性。紹介サイトの web host、staging の Killy server、デモ機、home cluster のどれが止まっても使えない（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed) の High availability）
- `localStorage` が使えないブラウザ（プライベートブラウズなど）で、再読み込みをまたいで同じ端末として扱うこと。そのときはページを開くたびに端末を登録し直す
- CORS を許可していないオリジンの Web ページから、Killy server の応答を読めるようにすること
- `/demo/` 以外の URL（`/demo/settings` など）を直接開き直したときの表示（Requirement 5 の 5.6 が扱う範囲を除く）
- Web 版のネイティブ並みの見た目の一致（ブラウザごとのフォント描画や動きの細かな違い）
- `mock/` を使うこと、変えること

## Requirements

### Requirement 1: Expo の Web 版が動くかを確かめる

**User Story:** Killy のチームとして、今のアプリが Web でも動くかを先に知りたい。大きな作り直しが要るなら、後続の作業を始める前に止めるため。

#### Acceptance Criteria

1. THE `mobile/` SHALL Web のビルドに必要な依存（Expo SDK 58 が勧めるもの）を持ち、`npx expo export --platform web` が成功する
2. THE チーム SHALL 次の 4 つを Web のビルドをブラウザで開いて確かめ、動いたか・動かなかったか・どう直すかを #129 に書く：battle 画面の描画（`react-native-svg` の絵と `react-native-reanimated` の動き）、`/app/v1/events` の SSE の受信、kill ボタンの 1 秒の長押し（マウスとタッチ）、足りない依存と警告
3. IF 動かないものがあり、大きな作り直しが要る THEN チーム SHALL 後続の issue に進まず、その内容を報告して止まる
4. THE iOS・Android の動作 SHALL 変わらず、既存の `cd mobile && npm run typecheck && npm run lint && npm test` が通る

### Requirement 2: Killy server が許可したオリジンにだけ CORS を許可する

**User Story:** Killy のチームとして、Web 版のページ（別のオリジン）が staging の Killy server を呼べるようにしたい。ただし他のオリジンには開かず、production は今のままにするため。

#### Acceptance Criteria

1. THE Killy server SHALL 許可するオリジンを、起動時に環境変数 `KILLY_CORS_ORIGINS`（カンマ区切り）から読む
2. WHERE `KILLY_CORS_ORIGINS` が未設定または空である THE Killy server SHALL どの応答にも CORS のヘッダ（`Access-Control-*` と、その理由の `Vary: Origin`）を付けない
3. IF `KILLY_CORS_ORIGINS` の項目が、スキーム・ホスト・（あれば）ポートだけの origin でない（`*` である、パスや空のホストを持つなど） THEN Killy server SHALL 起動に失敗する。項目の前後の空白と末尾の `/` は取り除いて読む
4. WHEN `Origin` が許可したオリジンの 1 つと完全に一致する要求が `/app/v1/` の下に届く THEN Killy server SHALL 応答に `Access-Control-Allow-Origin` としてそのオリジンと、`Vary: Origin` を付ける。成功の応答、`401`・`429` などのエラーの応答、SSE の `GET /app/v1/events` の応答のどれにも付ける
5. WHEN 許可したオリジンから、preflight（`Origin` と `Access-Control-Request-Method` を持つ `OPTIONS`）が `/app/v1/` の下に届く THEN Killy server SHALL `204` で、`Access-Control-Allow-Origin`（そのオリジン）・`Access-Control-Allow-Methods: GET, POST, PATCH, DELETE`・`Access-Control-Allow-Headers: Authorization, Content-Type`・`Access-Control-Max-Age` を返し、認証にも回数制限にも handler にも進まない
6. IF 要求の `Origin` が許可したオリジンのどれとも一致しない、または `Origin` が無い THEN Killy server SHALL `Access-Control-Allow-*` を付けない。`Access-Control-Allow-Origin` に `*` を返すことはない。許可していないオリジンの preflight には `204` も返さない（今の応答のまま）
7. THE Killy server SHALL `Access-Control-Allow-Credentials` を返さない
8. THE Killy server SHALL CORS を `/app/v1/` の下にだけ付け、agent の API（`/agent/v1/`）には付けない
9. THE CORS SHALL 認証と回数制限を変えない。許可したオリジンからの、token の無い要求や誤った token の要求は今までどおり `401` である
10. THE `deploy/overlays/stg/` SHALL `KILLY_CORS_ORIGINS=https://killy.sumire-sakamoto.co.jp` を持ち、`deploy/overlays/prd/` は `KILLY_CORS_ORIGINS` を持たない
11. THE `docs/ARCHITECTURE.md` と `docs/ARCHITECTURE.ja.md` SHALL CORS の設定と許可するものを書く
12. THE 審査が始まる前に、staging の image SHALL 更新される（人が行い、PR 本文に期限を書く）

### Requirement 3: Web では端末の保存先を localStorage にし、通知とカメラを出さない

**User Story:** 審査員として、再読み込みしても同じ端末として Killy を使いたい。ブラウザで動かない機能は出ないでほしい。

#### Acceptance Criteria

1. WHILE Web である THE app SHALL device token を `localStorage` の `device_token.<接続先 URL を整えたもの>` に保存し、次の起動でそのキーから読む。キーは native と同じ規則（`tokenKey`）で作る
2. WHILE Web である THE app SHALL 言語の設定と通知の設定を `localStorage` に保存し、読み出す。値が `ja`・`en`・`on`・`off` のどれでもなければ、保存されていないものとして扱う
3. IF `localStorage` が使えない（無い、または読み書きで例外になる） THEN app SHALL 落ちず、token は読み出しで「無し」、保存では何もしない。そのページの間は、登録した token をメモリで使って動く
4. WHILE Web である THE app SHALL OS の通知の許可を確かめず、許可を求めず、push token も取らず、Killy server に push token を送らない
5. WHILE Web である THE Settings SHALL 通知のスイッチと、その許可の案内を出さない
6. WHILE Web である THE Add server sheet SHALL QR の読み取りのボタンを出さず、カメラの許可を求めない
7. THE iOS・Android の起動と画面 SHALL 変わらず、`expo-secure-store`・`expo-notifications`・`expo-camera` を今までどおり使う
8. THE Web のビルド SHALL `expo-secure-store` を使う保存先と `expo-notifications` を使う実装を含まない（`.web.ts` で置き換える）
9. WHILE Web である THE kill ボタン SHALL 長押しで文字を選択させず（`user-select: none`）、長押しのメニュー（`contextmenu`）を出さない。iOS・Android には付けない

### Requirement 4: Web 版はデモ専用で、開くとデモの battle 画面を出す

**User Story:** 審査員として、ブラウザで URL を開くだけで、デモのゾンビと戦いたい。コードの入力や設定は要らない。

#### Acceptance Criteria

1. THE `config.ts` SHALL `EXPO_PUBLIC_KILLY_WEB_DEMO` が `1` のときだけ Web のデモ専用を有効とする（`demoEnabled` と同じ形。ほかの値や未設定は無効）
2. WHILE Web のデモ専用が有効である WHEN 端末の登録が済んだ THEN app SHALL `POST /app/v1/demo` を 1 回呼び、battle 画面を出す。起動のたびに 1 回呼ぶ（Killy server は同じ端末の 2 回目以降を、紐付けも表示名も変えずに受ける）
3. WHILE Web のデモ専用が有効である THE app SHALL `Platform.OS` が Web でないとき、この動きをしない（native の build は、この変数が設定されていても変わらない）
4. WHILE Web のデモ専用が有効である THE Settings SHALL Add server（コードの入力）を出さない。サーバーが 1 つも無い間、battle 画面は「設定からサーバーを追加する」案内（Add server が無いので指せない）を出さない。ペアリングの link（`/pair`）でも、コードの入力の sheet を開かない
5. IF 端末の登録が失敗した（`429`、届かない、そのほか） THEN app SHALL battle 画面に、理由を system text で出し、`POST /app/v1/demo` を呼ばない。文言は既存の `deviceRateLimited`・`deviceNotRegistered` を使う
6. IF `POST /app/v1/demo` が失敗した THEN app SHALL battle 画面に、理由を system text で出す。文言は Add server の「デモサーバーを試す」と同じ表（`ClaimError` の種類ごと：`demo_not_configured` は `demoNotConfigured`、届かないは `pairUnreachable`、そのほかは `pairFailed`）を使い、新しい文言を足さない。再読み込みでやり直せる。サーバーが 1 つでもあれば、この理由は出さない（返事が届かなかっただけで、追加されていることがある）
7. WHILE Web のデモ専用が有効で追加が済んだ THE app SHALL 公開デモの親を battle の列に出し、kill ボタンの 1 秒の長押しで kill できる。1 秒より前に離すと何も送らない
8. WHEN 他の人が同じデモの親を先に kill した THEN app SHALL その親を画面から消す。これは今のデモ（PRD UC6）と同じ動きである
9. WHILE Web である WHEN 画面の幅が 430px より広い THEN app SHALL 幅 430px の枠を中央に置き、その中に画面を出す。430px 以下のときは、画面いっぱいに出す。Modal の sheet も、枠と同じ幅にとどめる
10. THE Web のデモ専用 SHALL Killy の文言を system text にとどめ、新しい台詞を足さない（[killy-character](../../steering/killy-character.md)）
11. THE kill の安全確認（agent の `not_demo`）と回数制限 SHALL 今のまま使い、server と agent にデモ用の特別扱いを足さない

### Requirement 5: 紹介サイトの /demo/ に配置する

**User Story:** Killy のチームとして、Web 版を今の紹介サイトの配布の経路で出したい。cluster には何も足さずに済ませるため。

#### Acceptance Criteria

1. THE 配置の workflow SHALL `EXPO_PUBLIC_KILLY_SERVER_URL=https://stg-killy.sumire.pink` と `EXPO_PUBLIC_KILLY_WEB_DEMO=1` で `npx expo export --platform web` を実行し、`expo.experiments.baseUrl` を `/demo`、`web.output` を `static` にして、出力を `out/demo/` に置く
2. THE ページ SHALL 開いたとき、`/demo/` の下の JS とアセットだけを読み、サイトの外へは Killy server の `https://stg-killy.sumire.pink` にだけ要求する（`@expo-google-fonts` の JS には `fonts.gstatic.com` の文字列があるが、実行時には読まれない。出力の文字列ではなく実行時の要求で確かめる）
3. WHEN `mobile/**` が `main` で変わる THEN `deploy-site.yml` SHALL 走る
4. IF ビルドが失敗する THEN workflow SHALL 何もアップロードしない
5. THE 最後の確認 SHALL `/demo/` が 200 を返すことを含み、既存の確認（`/`・`/privacy/`・`/support/`・`/download/*` ほか）は変わらず通る
6. THE `/demo/` SHALL `.htaccess`（`sumiredc/sumire-sakamoto` が持つ）の書き換えを要らない形で配られる。route ごとの html（`settings.html`）は `settings/index.html` にも置き、`/demo/settings` と `/demo/settings/` を直接開いても（再読み込みでも）Settings が開く
7. THE workflow SHALL GitHub-hosted runner（`ubuntu-24.04`）のままで、外部の action は既存と同じくコミット SHA で固定し、秘密情報を足さない
8. WHEN `main` にマージされる THEN `https://killy.sumire-sakamoto.co.jp/demo/` SHALL Requirement 4 の動きになる

### Requirement 6: 文書が Web 版を書いている

**User Story:** 審査員と運用者として、Web 版の場所と、ブラウザに何が残るかを文書で知りたい。

#### Acceptance Criteria

1. THE `README.md` の「Try the public demo」 SHALL Web 版の URL と使い方を書く
2. THE `docs/PRIVACY.md` と `docs/PRIVACY.ja.md` SHALL Web 版が device token をブラウザの `localStorage` に置くことと、消し方を書く。`site/privacy/` は `go run ./tools/privacypage` で作り直し、`go test ./...` が通る
3. THE `docs/ARCHITECTURE.md` と `docs/ARCHITECTURE.ja.md` の Demo environment と Distribution SHALL Web 版を書く
4. THE `docs/PRD.md` の UC6 と Distribution SHALL Web 版を含む。Web 版もデモ専用で、staging の Killy server にだけつながることを書く
5. THE 他の文書 SHALL Web 版と食い違う記述（デモのボタンを出すのは審査員向けのネイティブの build だけ、など）を残さない
6. THE PR 本文 SHALL 紹介サイト（`site/index.html`）から `/demo/` への導線を足すかの提案を含む。導線そのものはこの spec の範囲にない
