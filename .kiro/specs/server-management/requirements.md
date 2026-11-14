# Requirements: server-management

## Introduction

追加したサーバーを phone から扱う部分。表示名の変更・通知の停止・削除、命令が agent に届いて戻るかの確認、`killy agent pair` の QR を読み取ってのサーバーの追加を作る。これで、M1 で作った Settings の Servers の行から server sheet が開き、Add server sheet でコードを手で打たずに追加できる。

流れの正は [PRD: UC1](../../../docs/PRD.md#uc1-add-a-server)・[UC4](../../../docs/PRD.md#uc4-check-the-connection-to-a-server)・[UC5](../../../docs/PRD.md#uc5-manage-a-server)、画面の正は [DESIGN: Server sheet](../../../docs/DESIGN.md#server-sheet)・[Add server sheet](../../../docs/DESIGN.md#add-server-sheet)・[When the Killy server cannot be reached](../../../docs/DESIGN.md#when-the-killy-server-cannot-be-reached)、通信の正は [ARCHITECTURE: Commands](../../../docs/ARCHITECTURE.md#commands) と [Pairing](../../../docs/ARCHITECTURE.md#pairing)、要求と応答の形は [openapi.yaml](../../../docs/openapi.yaml)。ここには確かめる基準だけを書く。

対応する issue：#16（Requirement 1）、#17（Requirement 2）、#18（Requirement 3）。

## 保証しないこと

- 通知を送る処理（#19）と push token の登録（#20）。この spec は、サーバーごとの通知の設定を保存するところまで
- アプリ全体の通知の switch（#20）と言語の切り替え（#21）、「デモサーバーを試す」（#24）
- 常時の接続表示と、応答の時間（round trip）・最後の受信の時刻（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed)）
- 他の phone の紐付けの削除、agent の停止、agent の登録の削除。削除で外れるのは、この device と agent の紐付けだけ
- QR を読み取っただけでのサーバーの追加。読み取ったコードは入力欄に入り、追加は「追加する」で行う
- Killy の絵と反応（追加できたときの 🤍 など。M3、#27）
- 署名付きの kill の命令（[PRD: Should-have](../../../docs/PRD.md#should-have)）

## Requirements

### Requirement 1: サーバーの表示名の変更・通知の停止・削除

**User Story:** operator として、追加したサーバーの表示名を変え、サーバーごとに通知を止め、要らなくなったサーバーを自分の phone から外したい。同じサーバーを紐付けた他の phone には影響させずに。

#### Acceptance Criteria

1. WHEN 紐付いたサーバーへの `PATCH /app/v1/servers/{id}` `{name?, notify?}` が届く THEN Killy server SHALL その device と agent の紐付けだけを変えて `204` を返し、同じ agent を紐付けた別の device の表示名と通知の設定を変えない
2. THE Killy server SHALL `name` を claim と同じ規則で扱う：前後の空白を除いて保存し、無い・空・空白だけの `name` では表示名を変えない
3. IF `name` が 100 文字を超える THEN Killy server SHALL `400`（`invalid_request`）を返し、表示名も通知の設定も変えない
4. WHEN `notify=false` に変わる THEN Killy server SHALL その device の `state` にサーバーとその親を残す
5. WHEN 紐付いたサーバーへの `DELETE /app/v1/servers/{id}` が届く THEN Killy server SHALL その device と agent の紐付けだけを外して `204` を返す。その device の `state` からサーバーと親が消え、同じ agent を紐付けた別の device の `state` には残る。agent には何も送らない
6. IF サーバーがその device に紐付いていない（存在しない ID・UUID の形でない ID を含む） THEN Killy server SHALL `PATCH` にも `DELETE` にも `404`（`server_not_found`）を返し、何も変えない
7. WHEN `PATCH` か `DELETE` が成功する THEN Killy server SHALL その device の開いている stream にだけ新しい `state` を送る
8. IF device token が不正である THEN Killy server SHALL `401` を返し、何も変えない
9. THE Killy server SHALL 表示名・通知の設定・紐付けの削除を DB に保存し、Killy server の再起動の後も保つ
10. THE Settings の Servers の行 SHALL 押せて、そのサーバーの server sheet を開く
11. THE server sheet SHALL 表示名の入力欄、ホスト名の行、このサーバーの通知の switch、削除のボタン、「完了」を持つ。ホスト名の行には `state` の `hostname` を出す
12. WHEN 通知の switch が切り替えられる THEN app SHALL その時点で `PATCH` `{notify}` を送る
13. WHEN 「完了」が押される THEN app SHALL 前後の空白を除いた表示名が空でなく、今の表示名と違うときだけ `PATCH` `{name}` を送り、成功したら sheet を閉じる。送るものが無ければ、何も送らずに閉じる
14. WHEN 削除のボタンが 1 回目に押される THEN app SHALL 何も送らず、ボタンを確認の表示（「もう一度押すと削除します」）に変える。2 回目に押されたら `DELETE` を送り、成功したら sheet を閉じる
15. IF `PATCH` か `DELETE` に応答が無い、またはエラーが返る THEN app SHALL sheet を閉じず、その操作をした欄の下に赤い行で理由を出す
16. WHILE そのサーバーの通知が off である THE battle 画面 SHALL そのサーバーの親を列に入れ続ける

### Requirement 2: サーバーへの接続を確認し、失敗した箇所を示す

**User Story:** operator として、サーバーごとに、命令が agent に届いて戻ってくるかを確かめ、失敗したならどこで失敗したかを知りたい。常時の接続表示が無くても、kill が届くかを確かめられるようにするため。

#### Acceptance Criteria

1. WHEN 紐付いたサーバーへの `POST /app/v1/servers/{id}/check` が届き、agent の命令の stream がある THEN Killy server SHALL その stream に `ping {id}` を送り、送ってから 5 秒より前に `pong` が届けば `{result: ok, version}` を返す
2. IF agent の命令の stream が無い THEN Killy server SHALL ping を送らずに `{result: agent_disconnected}` を返す
3. IF ping を送ってから 5 秒より前に `pong` が届かない（ちょうど 5 秒に届いたものを含む）、または stream はあるが命令を受け取らず ping を送れない THEN Killy server SHALL `{result: no_response}` を返す
4. WHEN 同じサーバーの前の確認から 2 秒以内（ちょうど 2 秒を含む）に確認が届く THEN Killy server SHALL ping を送らず、実行中の確認の結果か、直前の確認の結果を返す。2 秒を過ぎた確認では ping を送る
5. THE check の応答 SHALL `result` と、`ok` のときの agent の版だけを持ち、応答の時間・最後の受信の時刻を含めない
6. THE Killy server SHALL `pong` を ping の待ちにだけ、kill の結果を kill の待ちにだけ渡す。待っている kill の ID を付けた `pong` も、待っている ping の ID を付けた kill の結果も受け付けず、どちらの待ちも終わらせない
7. THE Killy server SHALL `pong` を、ping を送った agent からの `POST /agent/v1/results` だけから受け付ける
8. IF サーバーがその device に紐付いていない THEN Killy server SHALL `404`（`server_not_found`）を返し、ping を送らない
9. IF device token が不正である THEN Killy server SHALL `401` を返す
10. IF DB に届かない THEN Killy server SHALL `503`（`unavailable`）を返す
11. WHEN agent が命令の stream で `ping {id}` を受け取る THEN agent SHALL 走っている kill を待たずに `POST /agent/v1/results` `{id, type: pong, version}` を送る
12. THE agent SHALL `pong` に、`killy agent run` が持つ版（ビルド時に入れた値）を入れる
13. THE agent SHALL ping を受けても Linux サーバーの上で何も変えない（signal を送らず、`/proc` も記録ファイルも触らない）
14. THE server sheet の Connection の欄 SHALL 「確認する」のボタンを持ち、一度結果が出たら「もう一度確認」に変える。確認の間は問い合わせ中の表示を出す
15. WHEN 確認の結果が出る THEN app SHALL 次の表どおりに出す

    | 結果 | 表示 |
    |---|---|
    | `ok` | 成功として、接続できたことと agent の版 |
    | `agent_disconnected` | 失敗として、agent から応答が無いこと、サーバーで killy agent が動いているかを確かめること |
    | `no_response` | 失敗として、命令が agent に届いていないこと、agent を再起動すること |
    | phone の要求に応答が無い | 失敗として、通信を確かめること |
    | `401`・`404`・`503`、または JSON でない応答 | 失敗として、確認できなかったこと |

### Requirement 3: `killy agent pair` の QR を app で読み取ってサーバーを追加する

**User Story:** operator として、6 文字のコードを手で打つ代わりに、`killy agent pair` が出す QR を phone で読み取ってサーバーを追加したい。

#### Acceptance Criteria

1. WHEN `killy agent pair` がコードを得る THEN agent SHALL コードと agent の ID と一緒に、`killy://pair?code=<code>&server=<url>` の QR を端末に表示する。query は `url.Values` で組み立て、`server` を percent-encode する
2. THE QR の `server` SHALL `agent.json` に保存した Killy server の URL（初回は `--server` を正規化した値）にする。`--server` を省いた 2 回目以降の実行でも同じ URL になる
3. THE agent SHALL QR に agent token を含めない
4. THE Add server sheet SHALL 「QR コードを読み取る」のボタンを持ち、押されたら `expo-camera` で QR を読み取る
5. WHEN 読み取った文字列が受け付けられる `killy://pair` の URL である THEN app SHALL そのコードをコードの入力欄に入れ、claim は送らない。追加は「追加する」で行う
6. IF 読み取った文字列が `killy://pair` 以外の URL、コードの無い URL、コードが英大文字と数字の 6 文字でない URL のどれかである THEN app SHALL それを受け付けず、コードの入力欄を変えない
7. IF URL の `server` が app の接続先と違う THEN app SHALL 別の Killy server の QR であることをエラーとして出し、Killy server への要求を 1 つも送らない
8. THE app SHALL `server` と接続先を、scheme と host を小文字にし、既定の port を省き、末尾の `/` を除いてから比べる。末尾の `/`・host の大文字・既定の port の有り無しだけが違う `server` は受け付ける
9. WHEN app が `killy://pair?...` の deep link で開かれる THEN app SHALL 6・7・8 と同じ確認をした上で、Add server sheet を開いてコードの入力欄にコードを入れる
10. IF カメラの権限が無い THEN app SHALL コードの入力で追加できることを案内し、コードの入力欄は使えるままにする
11. THE app SHALL この Requirement で足すエラーと案内の文を、日本語と英語の翻訳ファイルの両方に持つ
