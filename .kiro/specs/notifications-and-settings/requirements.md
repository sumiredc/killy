# Requirements: notifications-and-settings

## Introduction

Linux サーバーに新しいゾンビの親が現れたことを、app を閉じていても phone に push で知らせる部分と、app の設定（アプリ全体の通知と言語）。EAS の project と push の認証情報を用意し、Killy server が親ごとに 1 回だけ push を送り、app が push token を登録して通知の設定と通知からの起動を扱い、言語を選べるようにする。

流れの正は [PRD: UC2](../../../docs/PRD.md#uc2-get-notified-of-zombies)・[UC8](../../../docs/PRD.md#uc8-app-settings)、保証の正は [PRD: Duplicates and identity](../../../docs/PRD.md#duplicates-and-identity)・[Timeliness](../../../docs/PRD.md#timeliness)、送る規則の正は [ARCHITECTURE: Notifications](../../../docs/ARCHITECTURE.md#notifications) と [State](../../../docs/ARCHITECTURE.md#state)、文面と画面の正は [DESIGN: Push notification](../../../docs/DESIGN.md#push-notification) と [Settings tab](../../../docs/DESIGN.md#settings-tab)、要求と応答の形は [openapi.yaml](../../../docs/openapi.yaml) の `PATCH /app/v1/devices/me`。ここには確かめる基準だけを書く。

対応する issue：#65（Requirement 1）、#19（Requirement 2）、#20（Requirement 3）、#21（Requirement 4）。

## 保証しないこと

- push receipt を取りに行くこと。receipt にだけ出る `DeviceNotRegistered` には気づかず、その push token には新しい親が現れるたびに送り続ける（#19 の決定）
- 通知を必ず 1 回だけにすること。Expo Push Service が受け付けた直後、記録する前に Killy server が止まると、その 1 件は再起動の後にもう一度送られる（[PRD: Duplicates and identity](../../../docs/PRD.md#duplicates-and-identity) の例外）
- 端末に実際に表示されること。Expo が受け付けた後の配信は APNs・FCM と OS が行う
- 通知の文面を app の言語に合わせること。文面は DESIGN の 1 つの形（`zombie ×<n> on <server>`）で、Killy server は phone の言語を知らない
- 通知をタップしたときに、その親を battle の先頭にすること。battle は全サーバーの 1 列で、最も古く検出された親から戦う（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed) の「Choosing which server to fight」）
- app が前面にあるときの、app の中での通知の表示。battle 画面がすでに最新の状態で動いている
- Slack・Discord などへの通知、通知の履歴（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed)）
- サーバーごとの通知の switch の API と server sheet（#16）。この spec はその switch を読むだけ
- Killy server がアプリ全体の switch の値を app に返すこと。そのための API は無い（[openapi.yaml](../../../docs/openapi.yaml)）
- Killy server の deployment への `EXPO_ACCESS_TOKEN` の設定と、push security を有効にするか（#30・#31）
- 配布用の build、TestFlight と APK（#32）
- 日本語と英語以外の言語

## Requirements

### Requirement 1: EAS の project と push の認証情報を用意する

**User Story:** 開発者として、#20 で app が push token を取り、実機で push を受け取れるよう、EAS の project と push の認証情報が先にそろっていてほしい。

#### Acceptance Criteria

1. THE app の設定（`mobile/app.json`） SHALL `extra.eas.projectId` を持ち、`mobile/` で `npx expo config --type public` を実行した出力にその値が出る
2. THE EAS の project SHALL iOS の APNs の key と Android の FCM V1 の認証情報を持つ。sumire が `eas credentials` の画面などで確かめ、値を伏せて #65 にコメントする
3. THE app の設定 SHALL `android.googleServicesFile` で `google-services.json` を読む。リポジトリに入れるかは #65 の PR で決め、入れないときは development build を作る人がどこから受け取るかを PR 本文に書く
4. WHEN development build を作り直す THEN app SHALL iOS と Android で起動し、2 タブを切り替えられる
5. THE リポジトリ SHALL APNs の key・FCM の service account の key の値と、`ios/`・`android/` を含まない
6. THE app の型検査・lint・テスト SHALL 通る

### Requirement 2: Killy server が新しいゾンビの親を、親ごとに 1 回だけ push で通知する

**User Story:** operator として、app を閉じていても、Linux サーバーにゾンビの親が現れたら 30 秒以内に一度だけ知らせてほしい。同じ親で何度も鳴らないようにするため。

#### Acceptance Criteria

1. WHEN snapshot を受け付ける THEN Killy server SHALL その中の親ごとに、その agent に紐付いた device のうち、push token があり、アプリ全体の switch とその device でのサーバーの switch が両方 on で、まだその親を通知していないものに push を送る
2. WHILE 親が snapshot に報告され続ける THE Killy server SHALL その親を device ごとに 1 回だけ通知する
3. WHEN 同じ PID で start time が違う親が報告される THEN Killy server SHALL それを新しい親として通知する
4. THE Killy server SHALL 親と device の組を、Expo Push Service の push ticket の `status` が `ok` だったときにだけ通知済みとして PostgreSQL に記録し、push receipt は見ない
5. WHEN Killy server が再起動した後に、通知済みの親が報告される THEN Killy server SHALL その親を通知しない
6. IF 送信が失敗した THEN Killy server SHALL 通知済みを記録せず、後の snapshot で送り直し、成功したら記録する
7. IF 同じ親と device の組への送信が、最初の送信を含めて 4 回失敗した THEN Killy server SHALL それ以上送らず、通知済みとして記録する
8. THE Killy server SHALL push ticket の `DeviceNotRegistered` 以外のエラーを、その組の失敗として数え、要求そのものの失敗（通信の失敗、4xx・5xx）を、その要求に入れたすべての組の失敗として数える
9. IF push ticket が `DeviceNotRegistered` を返した THEN Killy server SHALL その device の push token を消し、以後その device に送らない。その組は通知済みにも失敗にも数えない
10. WHEN push token を消した device が `PATCH /app/v1/devices/me` で push token を登録し直す THEN Killy server SHALL その device にまた送る
11. THE Killy server SHALL 失敗の回数を親と device の組ごとにメモリで数え、親が snapshot から消えたらその親の回数を消す。再起動の後は 0 から数え、上限は再起動の後も働く
12. WHEN 親が snapshot から消える THEN Killy server SHALL その親の通知済みの記録を消し、同じ `(pid, starttime)` の親がまたゾンビを持って報告されたら、新しい回としてもう一度通知する
13. WHEN Killy server が止まっている間に親が消えていて、再起動後のその agent の最初の snapshot にその親が無い THEN Killy server SHALL その親の通知済みの記録を消し、その後また現れたら通知する
14. WHEN device がアプリ全体の switch かサーバーの switch を on に戻す、または親より後にその agent を紐付ける THEN Killy server SHALL その時点でゾンビを持っている親を、次の snapshot でその device に通知する
15. WHEN device が紐付けを外す（#16 の `DELETE /app/v1/servers/{id}`） THEN Killy server SHALL その device とその agent の通知済みの記録を消す。紐付け直した device は 14 の「後から紐付けた device」として扱う（M2 の spec の確認で sumire が決めた）
16. WHEN `PATCH /app/v1/devices/me` で push token が登録される THEN Killy server SHALL 同じ push token を持つ別の device から、その push token を消す（M2 の spec の確認で sumire が決めた）
17. THE push SHALL タイトル `Killy`、本文の 1 行目 `zombie ×<n> on <server>`、最後の行 `……！` だけ、の形にする。`<n>` はその親のゾンビの数、`<server>` はその device での表示名（[DESIGN: Push notification](../../../docs/DESIGN.md#push-notification)）
18. WHEN 同じ agent を紐付けた 2 台の device に同じ親を通知する THEN Killy server SHALL それぞれの push に、それぞれの device での表示名を入れる
19. THE Killy server SHALL snapshot の応答を push の送信で待たせない。Expo Push Service が遅くても、snapshot の応答は遅れない
20. THE Killy server SHALL snapshot を受け付けたらすぐに送り、次の snapshot を待たない（[PRD: Timeliness](../../../docs/PRD.md#timeliness) の 30 秒）
21. IF snapshot が古い `scanned_at` のために捨てられた THEN Killy server SHALL その snapshot で push を送らない
22. WHEN `PATCH /app/v1/devices/me` `{push_token?, notify?}` が届く THEN Killy server SHALL 送られた欄だけをその device に保存し、`204` を返す
23. IF `push_token` が空か 512 バイトを超える THEN Killy server SHALL `400`（`invalid_request`）を返し、何も変えない（`POST /app/v1/devices` と同じ規則）
24. IF device token が不正である THEN Killy server SHALL `401` を返し、何も変えない
25. THE Killy server SHALL `EXPO_ACCESS_TOKEN` が設定されていれば `Authorization: Bearer` に付けて送り、無ければ付けずに送る
26. THE Killy server SHALL 通知済みの記録を、PostgreSQL の migration（`internal/server/store/migrations/` の 4 つ目）で作る table に持つ

### Requirement 3: app で push token を登録し、通知の設定と通知からの起動を扱う

**User Story:** operator として、phone で通知を受け取るかを選び、届いた通知をタップしたら battle 画面を開きたい。

#### Acceptance Criteria

1. WHEN device の登録が済み、OS の通知の許可がまだ決まっていない THEN app SHALL 起動したときに OS に通知の許可を求める（M2 の spec の確認で sumire が決めた）
2. WHEN app が push token を取得した THEN app SHALL `PATCH /app/v1/devices/me` `{push_token}` で登録する
3. WHEN push token が変わった THEN app SHALL 新しい push token を登録し直す
4. WHEN app が起動する THEN app SHALL push token を取得できれば、変わっていなくても登録し直す
5. IF push token の登録が失敗した THEN app SHALL 次の起動か push token が変わったときに登録し直し（#20 の決定）、何も表示しない（M2 の spec の確認で sumire が決めた）
6. IF push token を取得できない（通知を許可していない） THEN app SHALL `PATCH /app/v1/devices/me` を送らず、サーバーの追加と battle の操作をそのまま使えるようにする
7. THE Settings の App のグループ SHALL 「通知」の行に、アプリ全体の switch と「ゾンビプロセスが見つかったら知らせる」の hint を出す（[DESIGN: Settings tab](../../../docs/DESIGN.md#settings-tab)）
8. WHEN アプリ全体の switch を切り替える THEN app SHALL `PATCH /app/v1/devices/me` `{notify}` を送り、成功したら値を `expo-secure-store` に保存する
9. WHEN app が起動する THEN app SHALL 保存したアプリ全体の switch の値で switch を出す。保存が無ければ on で出す。保存した値を読めないときも on で出す（M2 の spec の確認で sumire が決めた）
10. IF アプリ全体の switch の `PATCH` が失敗した（応答が無い、エラーが返る、device token がまだ無い） THEN app SHALL switch を元の値に戻し、その行の下に理由を出し、値を保存しない
11. IF OS が Killy の通知をブロックしている THEN Settings の「通知」の行 SHALL 「OS で通知がブロックされています」とシステム設定へのリンクを出し、switch を押せなくする
12. WHEN システム設定から app に戻る THEN app SHALL OS の通知の許可を確かめ直し、11 の表示と switch を合わせ、許可されていれば push token を取得して登録する（M2 の spec の確認で sumire が決めた）
13. WHEN 通知をタップする THEN app SHALL Battle タブを開く。app が裏にあるときも、動いていないときも開く
14. WHILE app が前面にある THE app SHALL 届いた push を表示しない
15. WHEN 紐付けた Linux サーバーにゾンビの親が現れる（agent と Killy server がつながっている） THEN 実機の phone SHALL 30 秒以内に通知を受け取る（[PRD: Timeliness](../../../docs/PRD.md#timeliness)）
16. THE app SHALL 通知の行の文言（ブロックの表示、リンク、失敗の理由）を、日本語と英語の翻訳ファイルに持つ。ブロックの表示は「OS で通知がブロックされています」/「Notifications are blocked by the OS」（[DESIGN: Settings tab](../../../docs/DESIGN.md#settings-tab)、#20）、Killy server に届かないときの理由は既存の `pairUnreachable` の文にする（M2 の spec の確認で sumire が決めた）

### Requirement 4: app の言語を日本語と英語から選べるようにする

**User Story:** operator として、app の言語を日本語と英語から選びたい。端末の言語と違う言語で使いたいときのため。

#### Acceptance Criteria

1. THE Settings の App のグループ SHALL 「言語」の行に、日本語 / English の切り替えを出す（[DESIGN: Settings tab](../../../docs/DESIGN.md#settings-tab)）
2. WHEN 言語を切り替える THEN app SHALL 開いている画面の文言（タブの名前を含む）をすぐにその言語に変える
3. WHEN 言語を切り替える THEN app SHALL 選んだ言語を `expo-secure-store` に保存する
4. WHEN app が起動し、保存した言語がある THEN app SHALL 端末の言語より保存した言語を優先する
5. IF 保存した言語が無い THEN app SHALL mobile-battle-slice の Requirement 1.5 の初期値（日本語の端末なら日本語、それ以外は英語）を使う
6. WHILE 保存した言語を読み終えていない THE app SHALL splash を出したままにし、画面を描かない
7. IF 保存した言語を読めない、または保存に失敗した THEN app SHALL 読めないときは 5 の初期値を使い、保存に失敗したときも切り替えた言語で表示を続ける。どちらも表示は出さない（M2 の spec の確認で sumire が決めた）
