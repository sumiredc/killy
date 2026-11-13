# Tasks: mobile-battle-slice

各トップレベルのタスクが 1 つの issue と 1 つの PR に当たる。テストは実装より先に書き、期待した理由で赤になることを確かめる。完了条件の正は各 issue。各 PR で `cd mobile && npm run typecheck && npm run lint && npm test` を通す。

- [x] 1. Expo app の土台を作る（#11）
  - [x] 1.1 `mobile/` に Expo（TypeScript、`expo-router` の tabs）を作り、型検査・lint・テストのコマンドをそろえる
    - _Requirements: 1.1, 1.2_
  - [x] 1.2 翻訳のテスト（2 言語のキーの一致、除外するキーを除いた `I18N` のキーを含む、端末の言語ごとの初期値）を書き、赤を確かめる
    - _Requirements: 1.3, 1.4, 1.5_
  - [x] 1.3 `I18N` を `ja.json`・`en.json` に移し、`t()` と初期の言語の処理を実装する
    - _Requirements: 1.3, 1.4, 1.5_
  - [x] 1.4 接続先の設定を読む処理と、無いときのエラーのテストを書き、実装する
    - _Requirements: 1.6, 1.7_
  - [x] 1.5 Design tokens とフォント、2 タブを置き、iOS と Android の development build で起動したスクリーンショットを PR 本文に載せる
    - _Requirements: 1.1, 1.2, 1.8_

- [ ] 2. コードでサーバーを追加し、状態を受け取る（#12。前提：#8、#9、#11）
  - [x] 2.1 `KillyClient` の interface と偽物、`backoff` と `serverRow`（切断・ゾンビあり・ゾンビなし、表示名）のテストを書き、赤を確かめる
    - _Requirements: 2.10, 2.11, 2.12, 2.13, 2.14_
  - [x] 2.2 device の登録（`429` を含む）、device token の保存、token が無い間の「追加する」のテストを書き、実装する
    - _Requirements: 2.1, 2.2, 2.3_
  - [x] 2.3 Add server sheet のテスト（入力欄、表示名、成功、`error` ごとの文言）を書き、実装する
    - _Requirements: 2.4, 2.5, 2.6, 2.7_
  - [x] 2.4 SSE の受信を `react-native-sse` か `expo/fetch` で作り、置き換え・切断中の表示・再接続のテストを通す。選んだ理由を PR 本文に書く
    - _Requirements: 2.8, 2.9, 2.10_
  - [x] 2.5 Settings の Servers の行を作り、実機でローカルの Killy server にサーバーを追加したこと（agent を止めた状態でも）を PR 本文に書く
    - _Requirements: 2.6, 2.11, 2.12, 2.13, 2.14_

- [ ] 3. battle 画面で長押しで kill する（#13。前提：#10、#12）
  - [ ] 3.1 `buildQueue`・`applyOutcome`・`allClear` のテスト（列の順と同じ時刻の順、切断の除外、新しい親、outcome の表の各行、隠す親、別のサーバーの同じ組、battle の区切りと All clear の数え方）を書き、赤を確かめる
    - _Requirements: 3.1, 3.2, 3.4, 3.8, 3.12, 3.13_
  - [ ] 3.2 純粋な関数を実装し、テストを緑にする
    - _Requirements: 3.1, 3.2, 3.4, 3.8, 3.12, 3.13_
  - [ ] 3.3 `useHoldToKill` と kill ID の生成のテストを書き、実装する
    - _Requirements: 3.5, 3.6_
  - [ ] 3.4 `KillyClient.kill` の送り直しと `429` のテストを書き、実装する
    - _Requirements: 3.9, 3.11_
  - [ ] 3.5 battle 画面（今の親、kill ボタン、info の行、ボタンの下の赤い行とボタンが押せるようになること、空の状態、All clear、Settings タブとバッジ）を作り、画面のテストを通す
    - _Requirements: 3.3, 3.7, 3.8, 3.10, 3.11, 3.13, 3.14, 3.15, 3.16, 3.17_
  - [ ] 3.6 実機で Linux 上の実際のゾンビの親を長押しで kill し、All clear まで進んだことを PR 本文に書く（M1 の完了の確認）
    - _Requirements: 3.5, 3.8, 3.13_
