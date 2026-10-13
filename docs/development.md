# 開発プロセス

## 開発手法: 受け入れ条件駆動 + TDD
1. **仕様を先に書く**: 新しい機能は、まず `docs/requirements.md` に要件の ID (FR-n) と受け入れ条件を追加する。仕様だけの PR にしてもよい。
2. **Red**: 失敗するテストを先に書く。
   - `internal/pokeapi`: `httptest.Server` を使った単体テスト
   - `internal/app`: fake の `Source` を使った単体テスト
   - UI: 受け入れ条件を `ui.NewTester` の操作テストにする
   - テスト名に要件の ID を入れる (例: `TestSearch_FR1_CaseInsensitive`)。テストの本文は Given / When / Then の順に並べる。
3. **Green**: テストが通る最小の実装を書く。
4. **Refactor**: テストが green のまま整理する。
5. **PR に対応を書く**: PR の本文に、満たした要件の ID と、それを確かめるテスト名を書く。

例外として、CI の YAML や見た目だけの調整は、テストを後から書くか省略してよい。その場合は理由を PR に書く。

## アーキテクチャ
依存は一方向にする: `main` → `internal/ui` → `internal/app` → `internal/pokeapi`

```
main.go              ウィンドウを作って起動するだけ
internal/
  pokeapi/           Client (タイムアウト、context 対応)、ディスクキャッシュ、API の型
  app/               Model (検索語、結果、選択、読み込み状態)、絞り込みの純関数、非同期ジョブ
  ui/                View(c *ui.Context, m *app.Model)
```

- `app` は API を次のインターフェースとして受け取る。本番では `pokeapi.Client` を渡し、テストでは fake を渡す。
  ```go
  type Source interface {
      List(ctx context.Context) ([]pokeapi.Entry, error)
      Get(ctx context.Context, name string) (*pokeapi.Pokemon, error)
  }
  ```
- 取得は goroutine で行い、結果は `Window.Update` で Model に反映する。ジョブには世代番号を付け、古い世代の結果は捨てる (FR-4)。
- MyGo 固有の注意点は `.agents/skills/mygo-maintenance/SKILL.md` を読む。要素や `*ui.Context` を状態や goroutine に持たせない。

## AI ループ (Claude Code の設定)
Claude が人の手を待たずに「書く → 検証 → 直す」を回せるように、速さの違う 3 段のループと安全柵を `.claude/` に置いている。

| ループ | いつ動くか | 仕組み |
|---|---|---|
| 内側 | `.go` を編集するたび | `.claude/hooks/post_edit.py` (PostToolUse): `gofmt -w` と、そのパッケージの `go vet` を実行する。問題があればその場で Claude に返す |
| 中間 | Claude がターンを終えようとしたとき | `.claude/hooks/stop_gate.py` (Stop): origin/main から Go 関連の変更があれば `scripts/check.sh` を実行する。失敗したら終了させずに修正を続けさせる (同じ変更につき最大 3 回。それでも通らなければユーザーに報告して止まる)。通った状態は `.git/claude-stop-gate/` に記録し、同じ状態では再実行しない |
| 外側 | 1 要件 = 1 PR | skill `/feature FR-n` (`.claude/skills/feature/`): 要件の確認 → ブランチ作成 → Red → Green → Refactor → `reviewer` subagent による自己レビュー → PR → `gh pr checks --watch` で CI が green になるまで直す |

- **CI の定義が正**: `scripts/check.sh` は、`.github/workflows/ci.yml` の `check` ジョブから `run:` を読み出し、同じ順で実行する (`internal/tools/cilocal`)。チェックを足すときは ci.yml だけを変えればよい。ci.yml がローカルで実行できない形 (`${{ }}` 式、別の shell など) になると、`cilocal` のテストが落ちる。
- **安全柵**: `.claude/hooks/guard_bash.py` (PreToolUse) が次の操作を拒否する: main への commit と push、force push、タグの push、`gh pr merge`、`gh release`、`gh repo edit`、書き込みを行う `gh api`。heredoc の本文はデータとして扱い、判定の対象にしない。この柵があるので、`git push` や `gh pr create` は許可の確認なしで実行できる (`.claude/settings.json` の allow)。判定のケースは `.claude/hooks/test_guard_bash.py` に置き、CI でも実行する。
- **レビュー**: `.claude/agents/reviewer.md` は読み取り専用。diff を、要件、MyGo のルール、並行性、層構造、DoD の順に確認する。
- hook は python3 で書いている (標準ライブラリだけを使う)。

## ブランチと PR
- `main` は常にリリースできる状態に保つ。main へ直接 push しない。
- 作業ブランチの名前: `feat/…`、`fix/…`、`docs/…`、`ci/…`、`chore/…`、`test/…`、`refactor/…`
- 1 つの PR では 1 つのテーマだけを扱う。
- 流れ: Claude がブランチを切って実装し、PR を出して CI を確認する。CI が green になったらユーザーがレビューし、**squash merge** する。

### 役割分担
| 操作 | 担当 |
|---|---|
| ブランチ作成、コミット、push、PR 作成、CI の確認と修正 | Claude |
| レビュー、マージ、タグの push、リポジトリ設定の変更 | ユーザー |

### main の保護
GitHub の無料プランでは、Private リポジトリにブランチ保護を設定できない。Private の間は上の運用ルールで守る。Public にしたら、「PR 必須」と「CI 必須」のブランチ保護 (ruleset) を設定する。

## コミットメッセージ
- 英語の [Conventional Commits](https://www.conventionalcommits.org/) で書く: `feat(pokeapi): add disk cache for name list`
- 主な type: `feat`、`fix`、`docs`、`test`、`refactor`、`ci`、`chore`
- squash merge するので、**PR のタイトルがそのまま main のコミットメッセージになる**。PR タイトルも同じ形式にする。
- コミットの作者には GitHub の noreply アドレスを使う (リポジトリの git 設定で指定する)。

## Definition of Done
PR は、次をすべて満たしてからレビューに出す。

- [ ] `gofmt -l .` の出力が空
- [ ] `go vet ./...`
- [ ] `go tool staticcheck ./...` (staticcheck は go.mod の `tool` でバージョンを固定する)。※ Go 1.27 に対応した v0.8.2 は公開から 2 週間待つため、2026-10-26 以降に追加する。それまではこの項目を省く
- [ ] `go tool mygo vet .`
- [ ] `go test -race -cover ./...`
- [ ] 関係する要件のテストを追加・更新した
- [ ] 必要に応じて docs と `CHANGELOG.md` の `[Unreleased]` を更新した
- [ ] CI が green

CI (`.github/workflows/ci.yml`) は ubuntu だけで回し、上の項目を確かめる。カバレッジは Job Summary に出す。macOS runner は Private リポジトリでは消費が大きいので、リリースのときだけ使う。

## バージョンとリリース
- [SemVer](https://semver.org/) に従う。1.0.0 までは、破壊的な変更があれば MINOR を上げる。
- 手順:
  1. `CHANGELOG.md` の `[Unreleased]` を `[X.Y.Z] - YYYY-MM-DD` にし、`mygo.json` の `version` を合わせる PR を出してマージする
  2. ユーザーが `vX.Y.Z` タグを main に push する
  3. `release.yml` が macOS (dmg) と Windows (zip) をビルドし、GitHub Release に添付する
  4. 成果物を Mac と Windows で起動して確認する

## Public にする前のチェックリスト
- [ ] 履歴全体にシークレットや個人情報がないことを確認する (`git log --all --format='%ae %ce' | sort -u` の結果が noreply だけ、など)
- [ ] LICENSE を追加する
- [ ] README にスクリーンショットとインストール手順 (Gatekeeper / SmartScreen の回避方法を含む) を載せる
- [ ] リポジトリの description と topics を設定する
- [ ] main のブランチ保護を設定する
