# Tasks: agent-process-control

各トップレベルのタスクが 1 つの issue と 1 つの PR に当たる。テストは実装より先に書き、期待した理由で赤になることを確かめる。完了条件の正は各 issue。

- [x] 1. `/proc` からゾンビを持つ親を読み取る（#3）
  - [x] 1.1 `go.mod`（Go 1.27、toolchain go1.27.3）と `cmd/killy` の 3 つのサブコマンドの入口を作る
    - _Requirements: 1.8_
  - [x] 1.2 `internal/protocol` に `Parent`・`Snapshot` を置く
    - _Requirements: 1.3_
  - [x] 1.3 `ParseStat` と `Scanner.Scan` のテストを fixture で書き、赤を確かめる
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7_
  - [x] 1.4 `ParseStat` と `Scanner` を実装し、テストを緑にする
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8_
  - [x] 1.5 `go vet ./...` と `go test ./...` を通し、Linux 上で実際のゾンビが snapshot に入ることを PR 本文に書く
    - _Requirements: 1.1, 1.3_

- [x] 2. kill の直前に対象を確かめ直してから SIGKILL を送る（#4）
  - [x] 2.1 `PidfdOps` の interface と偽物、`Killer` の最小のスタブを置く
    - _Requirements: 2.1_
  - [x] 2.2 outcome の表の各行のテスト（`SendKill` の回数と、Open → Stat → SendKill の順を含む）を書き、赤を確かめる
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12, 2.13, 2.14_
  - [x] 2.3 確かめ直しの手順を実装し、テストを緑にする
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12, 2.13, 2.14_
  - [x] 2.4 同じ親への同時の kill のテストを書き、親ごとの lock を実装する
    - _Requirements: 2.15_
  - [x] 2.5 `syscall` で `pidfd_open` と `pidfd_send_signal` を呼ぶ本番の `PidfdOps` と、`Available()` を作る
    - _Requirements: 2.1, 2.2, 2.17_
  - [x] 2.6 保護対象の 3 つの確認を 1 つずつ取り除いてテストが落ちることと、signal の呼び出しが 1 か所であること（grep）を確かめ、PR 本文に書く
    - _Requirements: 2.8, 2.9, 2.10, 2.16_
  - [x] 2.7 `go vet ./...` と `go test -race ./...` を通し、Linux 上で実際のゾンビの親を kill して `killed` になることを PR 本文に書く
    - _Requirements: 2.13_

- [ ] 3. 同じ kill ID で二度 signal を送らない（#5）
  - [ ] 3.1 `internal/protocol` の `IsUUIDv4` のテスト（v4・v1・variant 違い・大文字・形違い）と、拒んだ命令で signal を送らないテストを書き、実装する
    - _Requirements: 3.1, 3.8_
  - [ ] 3.2 処理済みの ID・同時の同じ ID・再起動後の同じ ID・ファイルが無い起動・最後の行が切れたファイル・mode 0600 で作られること・書き込みの失敗で繰り返さないことのテストを書き、赤を確かめる
    - _Requirements: 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_
  - [ ] 3.3 `OutcomeStore`（追記・起動時の読み込み・壊れた最後の行・mode 0600・書き込みの失敗で繰り返さないこと）と、ID ごとの待ち合わせを実装する
    - _Requirements: 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_
  - [ ] 3.4 記録を引く処理と待ち合わせを 1 つずつ取り除いてテストが落ちることを確かめ、PR 本文に書く
    - _Requirements: 3.2, 3.3_
  - [ ] 3.5 `go vet ./...` と `go test -race ./...` を通す
    - _Requirements: 3.2_
