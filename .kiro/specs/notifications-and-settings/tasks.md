# Tasks: notifications-and-settings

各トップレベルのタスクが 1 つの issue と 1 つの PR に当たる。テストは実装より先に書き、期待した理由で赤になることを確かめる。完了条件の正は各 issue。Go のテストは PostgreSQL を使わず `internal/server/memstore` で書き、Expo への送信は `push.Sender` の偽物に差し替える。app の PR では `cd mobile && npm run typecheck && npm run lint && npm test` を通す。

順序は #65 → #19 → #20。#21 の前提は #11 だけなので、#19・#20 と並行して進められる。Settings の App のグループと `mobile/src/prefs.ts` は、#20 と #21 の先に着手したほうが作る。

- [x] 1. EAS の project と push の認証情報を用意する（#65。前提：#11）
  - [x] 1.1 （sumire）`mobile/` で `eas init` を実行し、`extra.eas.projectId` を `app.json` に入れる。`npx expo config --type public` の出力にその値が出ることを PR 本文に書く
    - _Requirements: 1.1_
  - [x] 1.2 （sumire）APNs の key と FCM V1 の認証情報を `eas credentials` で EAS に登録し、値を伏せて #65 にコメントする。FCM の service account の JSON は手元から消すか `mobile/` の外に置く
    - _Requirements: 1.2, 1.5_
  - [x] 1.3 `app.json` の `android.googleServicesFile` から `google-services.json` を読めるようにし、リポジトリに入れるかを決めて PR 本文に書く。入れないときは、development build を作る人がどこから受け取るかを書く
    - _Requirements: 1.3_
  - [x] 1.4 `npx expo prebuild --clean` で development build を作り直し、iOS と Android で起動して 2 タブを切り替えられたことを PR 本文に書く
    - _Requirements: 1.4_
  - [x] 1.5 `git status` で `ios/`・`android/` と認証情報のファイルが入っていないことを確かめ、型検査・lint・テストを通す
    - _Requirements: 1.5, 1.6_

- [x] 2. Killy server が新しいゾンビの親を、親ごとに 1 回だけ push で通知する（#19。前提：#16。#15・#38・#39・#43 は済み）
  - [x] 2.1 migration `00004_notified_parents.sql`（first_detections と links への `ON DELETE CASCADE`）と、`store.Store` と `memstore` の `UpdateDevice`・`PushTargets`・`NotifiedOf`・`RecordNotified`・`ClearPushToken` の最小のスタブを置く。memstore の `DeviceRow` に `Notify` を足す
    - _Requirements: 2.12, 2.13, 2.15, 2.26_
  - [x] 2.2 `PATCH /app/v1/devices/me` のテスト（`push_token` と `notify` を送った欄だけ保存して `204`、`{}`、空と 513 バイトの `push_token` が `400` で何も変えない、`401`、同じ push token を持つ別の device）を書き、赤を確かめてから実装する
    - _Requirements: 2.16, 2.22, 2.23, 2.24_
  - [x] 2.3 `push.Sender` の偽物と `push.Notifier` の最小のスタブを置き、通知のテスト（何度報告されても 1 回・start time が違えば新しい親・作り直した Server で送られない・失敗で記録されず次の snapshot で送り直して成功で記録・最初を含めて 4 回で諦めて記録・要求そのものの失敗で全部の組を数える・`DeviceNotRegistered` で push token が消えて `PATCH` で登録し直すとまた送られる・消えた親の失敗の回数が消える・消えて戻った親と再起動中に消えた親がまた通知される・switch と push token の組み合わせ・on に戻した device と後から紐付けた device・紐付け直した device・文面と 2 台の device の表示名・捨てた snapshot で送らない）を書き、赤を確かめる
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12, 2.13, 2.14, 2.15, 2.17, 2.18, 2.21_
  - [x] 2.4 `push.Notifier` と store の SQL を実装し、`postSnapshot` から `Wake` を呼んで、テストを緑にする
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12, 2.13, 2.14, 2.15, 2.17, 2.18, 2.21_
  - [x] 2.5 止まったままの `push.Sender` で、snapshot の応答が遅れず、受け付けた snapshot ですぐに送り始めるテストを書き、緑にする
    - _Requirements: 2.19, 2.20_
  - [x] 2.6 `push.Expo` のテスト（宛先、`EXPO_ACCESS_TOKEN` の有無と `Authorization`、100 件ずつの分け方、ticket の順の対応、10 秒の待ち）を `httptest.Server` で書いて実装し、`cmd/killy/server.go` で `EXPO_ACCESS_TOKEN` を読む
    - _Requirements: 2.8, 2.25_
  - [x] 2.7 `docs/ARCHITECTURE.md` と `docs/ARCHITECTURE.ja.md` の Notifications に、「受け付けた」は push ticket の `ok` で receipt は見ないこと、receipt にだけ出る `DeviceNotRegistered` には気づかないことを書き足す
    - _Requirements: 2.4_
  - [x] 2.8 手元の PostgreSQL（`make db-up`）に向けて `killy server` を動かし、migration が流れること、通知済みの記録が再起動後も残ること、親が消えると消えることを PR 本文に書く
    - _Requirements: 2.5, 2.12, 2.26_
  - [x] 2.9 `go vet ./...` と `go test -race ./...` を通す
    - _Requirements: 2.19_

- [x] 3. app で push token を登録し、通知の設定と通知からの起動を扱う（#20。前提：#19、#65。#15・#38・#43 は済み）
  - [x] 3.1 `npx expo install expo-notifications` で入れ、`app.json` の `plugins` に足す（`defaultChannel`）。`Deps` の `NotificationsDeps` とその偽物、`KillyClient.updateDevice`、`prefs` の最小のスタブを置き、今 `makeDeps` を差し替えているテストに新しい欄を足す
    - _Requirements: 3.2_
  - [x] 3.2 push token の登録のテスト（許可がまだ決まっていなければ求める、取得したら送る、変わったら送る、起動のたびに送る、失敗しても何も出さない、取れないときは送らずにサーバーの追加と battle ができる）を書き、赤を確かめてから実装する
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_
  - [x] 3.3 App のグループ（#21 が先に作っていなければ）と「通知」の行のテスト（hint、切り替えで `PATCH`、成功で保存して次の起動でも同じ値、保存が無ければ on、`PATCH` の失敗と device token が無いときに戻して理由を出す、読めないときは on）を書き、赤を確かめてから実装する。足した文言で翻訳のテストを通す
    - _Requirements: 3.7, 3.8, 3.9, 3.10, 3.16_
  - [x] 3.4 OS のブロックのテスト（文言とリンク、switch を押せない、`active` に戻ったら確かめ直して許可されていれば登録する）を書き、赤を確かめてから実装する
    - _Requirements: 3.11, 3.12_
  - [x] 3.5 通知のタップのテスト（裏にあるとき、動いていないとき、開いている sheet を閉じる）と、前面の push を表示しない handler の返り値のテストを書き、赤を確かめてから実装する
    - _Requirements: 3.13, 3.14_
  - [ ] 3.6 development build を作り直し、実機で #19 の Killy server につなぐ。Linux 上で `sh -c 'sleep 1 & exec sleep 600' &` でゾンビを作ってから 30 秒以内に通知が届き、タップで Battle タブが開いたことを、時刻付きで PR 本文に書く。app を終了させた状態からも、タップで Battle タブが開いたことを書く（#75 に移した。iOS の実機で確かめられる段階になってから確かめる。sumire が決めた）
    - _Requirements: 3.13, 3.15_
  - [ ] 3.7 実機で OS の通知をブロックしたときの Settings の表示を、スクリーンショット付きで PR 本文に書く（#75 に移した。iOS の実機で確かめられる段階になってから確かめる。sumire が決めた）
    - _Requirements: 3.11_
  - [x] 3.8 型検査・lint・テストを通す
    - _Requirements: 3.6_

- [x] 4. app の言語を日本語と英語から選べるようにする（#21。前提：#11。#15 は済み。#19・#20 と並行できる）
  - [x] 4.1 `prefs` の `language`（#20 が先に作っていなければ `mobile/src/prefs.ts` ごと）と、`I18nProvider` の `initial`・`save` の最小のスタブを置く
    - _Requirements: 4.3_
  - [x] 4.2 App のグループ（#20 が先に作っていなければ）と「言語」の行のテスト（日本語 / English の切り替え、切り替えるとタブの名前を含む文言が変わる、保存され次の起動で端末の言語より優先される、保存が無ければ日本語の端末で日本語・フランス語の端末で英語、読み書きの失敗）を書き、赤を確かめる
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.7_
  - [x] 4.3 保存した言語を読み終える前に `SplashScreen.hideAsync` が呼ばれず、画面が描かれないテストを書き、赤を確かめる
    - _Requirements: 4.6_
  - [x] 4.4 `RootLayout` で言語を読み、`I18nProvider` と言語の行を実装して、テストを緑にする
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7_
  - [x] 4.5 型検査・lint・テストを通す
    - _Requirements: 4.2_
