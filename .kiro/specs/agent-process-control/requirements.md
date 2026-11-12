# Requirements: agent-process-control

## Introduction

agent が Linux サーバーのゾンビとその親を `/proc` から読み取り、kill の命令を受けたときに、対象を確かめ直してから `SIGKILL` を送る部分。Killy の安全の保証（[PRD: Security](../../../docs/PRD.md#security)、[Duplicates and identity](../../../docs/PRD.md#duplicates-and-identity)）を担う。

この spec は Killy server との通信を含まない。報告は relay-and-pairing の Requirement 2、命令の受け口は Requirement 5 が扱う。手順の正は [ARCHITECTURE: Scanning](../../../docs/ARCHITECTURE.md#scanning) と [Kill procedure](../../../docs/ARCHITECTURE.md#kill-procedure) で、ここには確かめる基準だけを書く。

対応する issue：#3（Requirement 1）、#4（Requirement 2）、#5（Requirement 3）。

## 保証しないこと

- demo mode の確認（summon の cgroup）。#23 で扱う
- `SIGKILL` 以外の signal。作らない（[PRD: Not guaranteed](../../../docs/PRD.md#not-guaranteed)）
- 別の PID namespace のゾンビ

## Requirements

### Requirement 1: ゾンビを持つ親を読み取る

**User Story:** operator として、agent にサーバー上のゾンビとその親を正しく見つけてほしい。どの親を kill すればよいかが分かるようにするため。

#### Acceptance Criteria

1. WHEN 同じ親を持つゾンビが複数ある THEN agent SHALL それらを 1 つの親の下にまとめる
2. WHEN ゾンビの親が PID 1 である THEN agent SHALL その親を結果に含めない
3. WHEN 親を報告する THEN agent SHALL 親の PID と start time（`stat` の 22 番目の値）を含める
4. WHEN `stat` のコマンド名（`comm`）に空白や `)` が含まれる THEN agent SHALL ほかの値を取り違えずに読む
5. IF 走査の途中でプロセスが消えて `stat` が読めない THEN agent SHALL そのプロセスを飛ばし、走査全体を失敗させない
6. WHEN 親の `cmdline` を読む THEN agent SHALL NUL で区切られた引数を空白でつなぎ、末尾の NUL を除いて `cmd` にする
7. IF 親の `cmdline` が空である THEN agent SHALL `comm` を `cmd` に入れる
8. THE agent SHALL 走査を Go の標準ライブラリだけで行う

### Requirement 2: 確かめ直してから SIGKILL を送る

**User Story:** operator として、画面に出た親だけが kill されてほしい。別のプロセスや守るべきプロセスが巻き込まれないようにするため。

#### Acceptance Criteria

1. WHEN kill を処理する THEN agent SHALL 先に対象の pidfd を開き、その後に `/proc` で確かめ直し、その pidfd に `SIGKILL` を送る
2. IF pidfd が使えない（`pidfd_open` が `ENOSYS` か `EPERM`） THEN agent SHALL signal を送らず `refused` / `pidfd_unavailable` を返し、PID 指定の送信に切り替えない
3. IF `pidfd_open` の時点で対象が無い THEN agent SHALL signal を送らず `already_gone` を返す
4. IF 確かめ直しのときに対象の `stat` が無い THEN agent SHALL signal を送らず `already_gone` を返す
5. IF start time が命令の `starttime` と違う THEN agent SHALL signal を送らず `refused` / `pid_reused` を返す
6. IF 対象そのものが状態 `Z` である THEN agent SHALL signal を送らず `already_gone` を返す
7. IF 対象にゾンビの子がもういない THEN agent SHALL signal を送らず `refused` / `not_a_zombie_parent` を返す
8. IF 対象が PID 1 である THEN agent SHALL signal を送らず `refused` / `protected` を返す
9. IF 対象が agent 自身である THEN agent SHALL signal を送らず `refused` / `protected` を返す
10. IF 対象が agent の祖先である THEN agent SHALL signal を送らず `refused` / `protected` を返す。祖先は kill のたびに `/proc` から辿り直す
11. IF 確かめ直しの後、送る前に対象が終わっていた（送信が `ESRCH`） THEN agent SHALL `already_gone` を返し、別のプロセスに signal を届けない
12. IF 送信がそれ以外の理由で失敗した THEN agent SHALL `signal_failed` を返し、errno の名前（例 `EPERM`）を `reason` に入れる
13. WHEN 送信が成功し、確かめ直しのときにいたゾンビが 5 秒以内にすべて消えた THEN agent SHALL `killed` と回収された数を返す
14. WHEN 送信が成功し、5 秒たってもゾンビが残っている THEN agent SHALL `zombies_remain` を返す
15. WHEN 同じ `(ppid, starttime)` への kill が同時に届いた THEN agent SHALL 1 つずつ処理する。先の kill が `killed` なら、後の kill は 3・4・6 のどれかにより `already_gone` になり、signal は 1 回だけ送られる
16. THE agent SHALL signal を送る呼び出しを 1 か所だけに持ち、その signal は `SIGKILL` だけにする
17. THE agent SHALL kill の処理を Go の標準ライブラリだけで書き、pidfd の呼び出しは `syscall` パッケージで直接行う

### Requirement 3: 同じ kill ID で二度 signal を送らない

**User Story:** operator として、通信の都合で同じ kill が二度届いても、二度 kill されないでほしい。PID が再利用されていた場合に別のプロセスを kill しないようにするため。

#### Acceptance Criteria

1. IF kill ID が UUID v4（小文字の 8-4-4-4-12、version の桁が `4`、variant の bit が `10`）でない THEN agent SHALL その命令を無視し、signal を送らない
2. WHEN 処理済みの kill ID が届いた THEN agent SHALL `pidfd_open` も `/proc` の読み取りも行わずに、最初の outcome をそのまま返す
3. WHEN 同じ kill ID が同時に 2 回届いた THEN agent SHALL 確かめ直しと送信を 1 回だけ行い、2 つとも同じ outcome を返す
4. THE agent SHALL 最初の outcome を `/var/lib/killy/outcomes.jsonl`（追記のみ、mode 0600）に期限なしで保存する
5. WHEN agent が起動する THEN agent SHALL 記録ファイルを読み込み、再起動の前に処理した ID にも 2 を当てはめる。ファイルが無ければ空として始める
6. IF 記録ファイルの最後の行が途中で切れている THEN agent SHALL それより前の記録を読み込んで動き続ける
7. IF 記録の書き込みに失敗した THEN agent SHALL ログに出し、書き込みを繰り返さずに outcome を返す
8. THE system SHALL UUID v4 の判定を `internal/protocol` の `IsUUIDv4` の 1 つの関数で行い、Killy server（relay-and-pairing の Requirement 5.3）と同じ規則にする
