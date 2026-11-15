# Tasks: demo-environment

各トップレベルのタスクが 1 つの issue と 1 つの PR に当たる。テストは実装より先に書き、期待した理由で赤になることを確かめる。完了条件の正は各 issue。各 PR で `go vet ./...` と `go test -race ./...` を通し、app を変える PR では `cd mobile && npm run typecheck && npm run lint && npm test` も通す。

- [x] 1. killy summon がデモ用のゾンビの親を作り続ける（#22。前提：#7、#15、#38）
  - [x] 1.1 `internal/summon` に `Summoner`・`Process`・`Commands`・`MaxParents`・`Interval` の最小のスタブを置く
    - _Requirements: 1.1_
  - [x] 1.2 偽物の `Start` で、生きている親が 3 つのときの `Tick` が親を作らず、1 つ終わって 2 つになった後の `Tick` で作るテストと、`Pick` を差し替えて 2 つ以上の候補からコマンド名が選ばれるテストを書き、赤を確かめる
    - _Requirements: 1.2, 1.3, 1.4_
  - [x] 1.3 `Tick`・`Alive`・`Run` と、親を起動するたびの `Wait` による回収を実装し、テストを緑にする
    - _Requirements: 1.2, 1.3, 1.4, 1.6_
  - [x] 1.4 `//go:build linux` のテスト（`TestMain` が `KILLY_SUMMON_ROLE` で親とゾンビの役をする）で、本物の親にテストから `SIGKILL` を送り、親が回収されて test のプロセスの子として状態 `Z` で残らないことを確かめるテストを書き、赤を確かめる
    - _Requirements: 1.6_
  - [x] 1.5 `ExecStart`（`/proc/self/exe` を argv を変えて起動）、`RunParent`（ゾンビを作って `wait()` しない。数と上限は PR で決める。案は design）、`main` の役の振り分け、`killy summon` の入口（`every(ctx, summon.Interval)`）を実装し、1.4 を緑にする。親ごとのゾンビの数と上限を PR 本文に書く
    - _Requirements: 1.1, 1.3, 1.5, 1.6_
  - [x] 1.6 `deploy/systemd/killy-summon.service`（`/usr/local/bin/killy summon`）を作る
    - _Requirements: 1.7_
  - [x] 1.7 `go vet ./...` と `go test -race ./...` を通し、Linux 上で `killy-summon.service` の親の `/proc/<pid>/cgroup` の `0::` の行が `0::/system.slice/killy-summon.service` であることと、summon と `killy agent run` で Killy server の snapshot にデモの親が 3 つまで入ったことを PR 本文に書く
    - _Requirements: 1.8, 1.9_

- [x] 2. demo mode の agent は summon が作った親だけを kill する（#23。前提：#4、#22、#15、#38）
  - [x] 2.1 `internal/protocol` に `ReasonNotDemo` を、`internal/proc` に `SummonCgroup`・`ErrNoUnifiedCgroup`・`Scanner.OnlyCgroup`・`Scanner.InCgroup` の最小のスタブを、`kill.Killer` に `Demo` を置く
    - _Requirements: 2.2_
  - [x] 2.2 `world_test.go` に cgroup のファイルの中身・読み取りのエラー・cgroup を読む直前に親を消す差し込みを足し、demo mode の kill のテスト（別の system の service・同じ名前の user unit・名前に文字を足したもの・`0::` の行が無いもの・読めないもの・読めず親がもう無いもの・一致するもの）と、demo mode でない kill のテストを書き、赤を確かめる。`refused` と `already_gone` では `SendKill` が 0 回であることを見る
    - _Requirements: 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_
  - [x] 2.3 `Scanner.InCgroup` と `Killer.check` の cgroup の確認を実装し、テストを緑にする
    - _Requirements: 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.11_
  - [x] 2.4 `fakeRoot` に cgroup のファイルを足し、`OnlyCgroup` があるときに 2.2 と同じ cgroup の親が `Scan` に入らず、無いときに入るテストを書き、赤を確かめてから `Scan` の絞り込みを実装する
    - _Requirements: 2.9, 2.10_
  - [x] 2.5 `killy agent run --demo` を足し、`--demo` で `OnlyCgroup` と `Demo` の両方が設定され、無ければどちらも設定されないことを `cmd/killy` のテストで確かめる
    - _Requirements: 2.1_
  - [x] 2.6 `deploy/systemd/killy-agent-demo.service`（`/usr/local/bin/killy agent run --demo` を root で）を作り、cgroup が読めないときの扱いを ARCHITECTURE と ARCHITECTURE.ja.md の Kill procedure の 4 に書き足す
    - _Requirements: 2.4, 2.5, 2.12_
  - [x] 2.7 cgroup の確認を取り除くと 2.3 のテストが落ち、完全一致を末尾一致に変えると user unit のテストが、前方一致に変えると名前に文字を足したもののテストが落ちることを確かめ、PR 本文に書く
    - _Requirements: 2.3_
  - [x] 2.8 `go vet ./...` と `go test -race ./...` を通し、Linux 上で `killy-agent-demo.service` として agent を動かし、`sh -c 'sleep 1 & exec sleep 600' &` で作った親が snapshot に入らないことと、その親の PID と start time で `curl` から `POST /app/v1/kills` を送って `not_demo` が返り、そのプロセスが生きていることを PR 本文に書く
    - _Requirements: 2.10, 2.13_

- [x] 3. app の「デモサーバーを試す」で公開デモのサーバーを追加する（#24。前提：#12、#23、#15、#38、#43）
  - [x] 3.1 `POST /app/v1/demo` のテスト（紐付く・2 回呼んでも 1 つで表示名と通知の設定が変わらない・紐付けた device にだけ `state` が届く・未設定と存在しない agent で `404` の `demo_not_configured` と紐付けなし・`401`）を書き、赤を確かめる
    - _Requirements: 3.2, 3.3, 3.4, 3.5, 3.6_
  - [x] 3.2 `Server.DemoAgentID`、`addDemoServer`、`cmd/killy/server.go` での `KILLY_DEMO_AGENT_ID` の読み込みを実装し、テストを緑にする
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_
  - [x] 3.3 `internal/relaytest` の形で、2 台の device が `POST /app/v1/demo` で紐付けた同じデモの親に、ID の違う kill を順に送るときと同時に送るときのテストを書き、`killed` と `already_gone` が 1 つずつで signal が 1 回であることを確かめる
    - _Requirements: 3.14_
  - [x] 3.4 `KillyClient.addDemoServer` と Add server sheet のテスト（ボタンが出る・追加できた・`demo_not_configured`・届かない・`unauthorized`・`unavailable`・JSON でない応答・登録できていないときに何も送らない）を書き、赤を確かめる
    - _Requirements: 3.8, 3.9, 3.10, 3.11, 3.12_
  - [x] 3.5 `addDemoServer`、`ClaimErrorKind` の `demo_not_configured`、Add server sheet の「デモサーバーを試す」、`ja.json` と `en.json` の文言を実装し、テストを緑にする
    - _Requirements: 3.8, 3.9, 3.10, 3.11, 3.12, 3.13_
  - [x] 3.6 `go vet ./...` と `go test -race ./...` と、app の型検査・lint・テストを通し、Linux 上で `killy-agent-demo.service` と `killy summon` を動かし、ローカルの Killy server を使って、実機かエミュレータで「デモサーバーを試す」から公開デモを追加し、デモの親を kill したことを PR 本文に書く
    - _Requirements: 3.15_
