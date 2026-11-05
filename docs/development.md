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
main には ruleset で「PR 必須」と「CI 必須」を設定している (「リポジトリのルールと設定」を参照)。

## コミットメッセージ
- 英語の [Conventional Commits](https://www.conventionalcommits.org/) で書く: `feat(pokeapi): add disk cache for name list`
- 主な type: `feat`、`fix`、`docs`、`test`、`refactor`、`ci`、`chore`
- PR のタイトルは `pr-title.yml` (`scripts/check-pr-title.sh`) が検査する。type は上のほかに `perf`、`build`、`revert` も使える。タイトルを直すと検査が再実行される。
- squash merge するので、**PR のタイトルがそのまま main のコミットメッセージになる**。PR タイトルも同じ形式にする。
- コミットの作者には GitHub の noreply アドレスを使う。リポジトリの `.git/config` に加えて、dev container では環境変数 `GIT_CONFIG_*` (`.devcontainer/devcontainer.json` と `scripts/dev.sh`) で `user.email` と `safe.directory` を指定する。VS Code がホストの `~/.gitconfig` (個人のアドレスを含みうる) をコンテナに複製しても、環境変数の設定が優先される。2 か所の設定が食い違わないことは `scripts/test_devcontainer.py` が確かめる。コンテナを作り直す (Rebuild Container) と反映される。なお、環境変数の `user.email` は、このコンテナ内のほかのリポジトリの `user.email` も上書きする。別のアドレスでコミットしたいときは、`GIT_AUTHOR_EMAIL` と `GIT_COMMITTER_EMAIL` を指定する。

## Definition of Done
PR は、次をすべて満たしてからレビューに出す。

- [ ] `gofmt -l .` の出力が空
- [ ] `go vet ./...`
- [ ] `go tool staticcheck ./...` (staticcheck は go.mod の `tool` でバージョンを固定する。v0.8.2 から Go 1.27 に対応)
- [ ] `go tool mygo vet .`
- [ ] `go test -race -cover ./...`
- [ ] 関係する要件のテストを追加・更新した
- [ ] 必要に応じて docs と `CHANGELOG.md` の `[Unreleased]` を更新した
- [ ] CI が green

CI (`.github/workflows/ci.yml`) は ubuntu だけで回し、上の項目を確かめる。カバレッジは Job Summary に出し、パッケージごとの下限 (`.github/coverage-baseline.txt`) を `scripts/coverage_ratchet.py` で守らせる (0.5 ポイントまでの低下は許す)。カバレッジが上がったら、同じ PR で下限の数字も上げる。下げるときは理由を PR に書く。

macOS でのビルドは `macos-build.yml` が main への push と、アプリやビルドに関わるファイル (`*.go`、`go.mod`、`go.sum`、`mygo.json`、`resources/`、このビルドの workflow とスクリプト) を変えた PR で行う。docs だけの PR では走らせない (macOS runner は待ち時間が長いため。必須チェックにもしない) (Release と同じビルドと `scripts/verify-macos-app.sh` の検証)。リポジトリは Public なので macOS runner の利用は無料。ビルドしたアプリは `scripts/smoke-app.sh` で起動し、数秒生きていることを確かめる (クラッシュの検知。ウィンドウの中身は見ない)。dmg は artifact `macos-arm64-build` (保持 1 日) に残る。`scripts/fetch-build.sh` は今のところ Release のドライラン用で、この artifact は `gh run download <run> --name macos-arm64-build` で取る。

脆弱性の検査 (SCA) は別 workflow の `govulncheck.yml` が行う (PR、main、毎週月曜)。呼び出している関数に既知の脆弱性があると失敗する。標準ライブラリの脆弱性も対象なので、落ちたら Go のバージョンを上げる。必須チェックにするかは Settings でユーザーが決める。

静的解析 (SAST) は `codeql.yml` が行う (PR、main、毎週月曜)。結果は Security → Code scanning に出る。リポジトリの「Code scanning の default setup」は無効のままにする (有効だとこの workflow と二重になり、アップロードが失敗する)。

リリースを公開すると、`verify-release.yml` が公開された資産をダウンロードして、(1) dmg・`SHA256SUMS.txt`・SBOM がそろっていること、(2) チェックサム、(3) Release workflow による来歴 (attestation) を `scripts/verify-release.sh` で確かめる。Actions → Verify release → Run workflow で、公開済みのタグを指定して再確認もできる (来歴を付け始める前のリリースは失敗する)。

main、タグ、定期実行 (schedule)、リリース公開後の検証の workflow が失敗すると、`notify-failure.yml` が `ci-failure` ラベルの Issue を作る (同じ workflow とブランチの Issue が開いていればコメントを足す)。PR の実行とドライランは対象外。原因を直すか再実行して、Issue をクローズする。

workflow ファイル自体の検査は `workflow-lint.yml` が行う (`.github/` を変える PR)。actionlint は構文と式を、zizmor は権限、script injection、キャッシュ汚染、Dependabot の cooldown などを見る。actionlint のバージョンは Dependabot が追わないので、手で上げる。

依存の差分の検査は `dependency-review.yml` が PR で行う。追加・更新された依存に moderate 以上の脆弱性があるか、強いコピーレフト (GPL / AGPL) のライセンスだと失敗する。Dependabot の Go PR は notices のテストで落ちるので (「Dependabot」を参照)、この検査はそのやり直しの PR で働く。

## バージョンとリリース
- [SemVer](https://semver.org/) に従う。1.0.0 までは、破壊的な変更があれば MINOR を上げる。
- 手順:
  1. `CHANGELOG.md` の `[Unreleased]` を `[X.Y.Z] - YYYY-MM-DD` にし、`mygo.json` の `version` を合わせる PR を出してマージする
  2. ユーザーが `vX.Y.Z` タグを main に push する
  3. `release.yml` が動く (下記)。macOS (dmg) を GitHub Release の**下書き**に添付する (Windows は v0.1.0 の範囲外)
  4. ユーザーが下書きの内容 (ノート、dmg、`SHA256SUMS.txt`) を確認し、**公開する**
  5. 成果物を Mac で起動して確認する

### 成果物を Mac で試す
CI が作った成果物は、ブラウザを通さずに取得する。Chrome は署名のない dmg をブロックすることがあり、`gh` で保存したファイルには macOS の隔離属性も付かない。

```sh
scripts/fetch-build.sh                   # 最新の成功したドライラン
scripts/fetch-build.sh --branch feat/x   # ブランチを指定
scripts/fetch-build.sh --run 39088009828 # run を指定
```

- 取得先はプロジェクトの `out-artifacts/run-<ID>/` (Git 管理外)。プロジェクトのフォルダは Mac と共有されているので、Finder からそのまま dmg を開ける。
- チェックサムを検証し、ビルドしたブランチとコミットを `BUILD_INFO.txt` に残す。**何のビルドかは、時刻ではなくこのコミットで確かめる。**
- 成果物は 1 日で期限が切れる。切れていたら、Actions → Release → Run workflow でドライランをやり直す (ブランチを選べる)。
- 公開済みのリリースは `gh release download vX.Y.Z --pattern '*.dmg' --pattern SHA256SUMS.txt` で取得する (リリースの操作はユーザーが行う)。

### `release.yml` (CD)
タグ `v*` の push で起動する。公開は人が行うので、これは継続的デリバリー (Continuous Delivery) で、自動公開ではない。

| ジョブ | runner | 内容 |
|---|---|---|
| `verify` | ubuntu | タグが `mygo.json` の version と一致すること、`CHANGELOG.md` にその版の節があることを確認し、`scripts/check.sh` (CI の check ジョブと同じ) を実行する。落ちたら macOS runner は使わない |
| `package-macos` | macos-latest | `go tool mygo build -platform darwin/arm64` (アドホック署名、dmg) → `codesign --verify` と arm64 の確認 → `Pokedex-X.Y.Z-macos-arm64.dmg` と `SHA256SUMS.txt` を作成 → CHANGELOG の節をノートにした下書きリリースを作り、添付する |

- 権限は既定で `contents: read`。`contents: write` は `package-macos` だけに与える。
- Actions タブの「Release」→ Run workflow で、任意のブランチから**ドライラン**できる。チェック、ビルド、署名の検証までは同じで、リリースは作らず、dmg と `SHA256SUMS.txt` を run の Artifacts (保持 1 日) に残す。タグを打つ前に macOS runner の経路を試し、dmg を Mac で起動して確かめるのに使う。
- ジョブを再実行すると、下書きは残したまま、ファイルを置き換える (`--clobber`)。
- `v1.2.3-rc1` のようにハイフンを含むタグは prerelease になる。ただし version の検査は厳密なので、`mygo.json` の version も同じにする必要がある。
- `package-macos` は dmg と `SHA256SUMS.txt` の来歴 (attestation, `actions/attest-build-provenance`) を作る。署名のない (アドホック署名の) dmg が、この workflow のこのコミットから作られたことを `gh attestation verify <file> --repo naoya03708/pokedex-app` で確かめられる。公証の代わりにはならない (macOS の警告は消えない)。`id-token: write` と `attestations: write` はこのジョブだけに与えている。ドライランでも作る。
- `package-macos` は SBOM (`Pokedex-X.Y.Z-sbom.cdx.json`, CycloneDX。アプリが使う Go モジュールの一覧) も作り、下書きリリースに添付して、dmg と同じ来歴を付ける。生成は `cyclonedx-gomod` (バージョンは release.yml に固定。Dependabot は追わないので手で上げる)。
- Release の検証手順は `scripts/verify-macos-app.sh` で、`macos-build.yml` と共有している。
- アクションはコミット SHA で固定している。更新は ci.yml と同じ SHA にそろえる。

## リポジトリのルールと設定
リポジトリは Public。変更はユーザーが GitHub の Settings で行う (Claude には変更させない)。

### ルール
- 権利者のいる画像 (ポケモンのスプライトなど) をコミットしない。README のスクリーンショットにも使わない。アプリはスプライトを同梱せず、実行時に取得するだけにする。
- シークレットや個人情報をコミットしない。コミットの作者は noreply アドレスにする (「コミットメッセージ」を参照)。
- 依存を増減したら、`go run ./internal/tools/notices` で `resources/THIRD_PARTY_NOTICES.txt` を更新する (古いと CI のテストが落ちる)。

### 設定 (崩れていないかの基準)
- マージは squash のみ。マージ後にブランチを削除する。
- main の ruleset (Bypass list は空): PR 必須、必須チェック `check` (ブランチを最新に保つ)、force push と削除の禁止、直線的な履歴。
- Secret scanning と push protection、Dependabot alerts、Private vulnerability reporting は有効。
- Actions のトークンの既定は read。外部コントリビューターの PR のワークフローは、全員を承認制にする。
- 公開用の description と topics を設定している。

### Dependabot
`.github/dependabot.yml` が、Go モジュールと Actions を週次で更新する PR を出す。
- **Actions の PR**: ci.yml と release.yml の SHA とバージョンのコメントをまとめて更新する。CI が通れば、そのままレビューしてよい。
- **Go モジュールの PR**: `resources/THIRD_PARTY_NOTICES.txt` にバージョンが入っているので、Dependabot の PR は notices のテストで落ちる。そのPRはクローズし、Claude に同じ更新を依頼する (`go get`、`go mod tidy`、`go run ./internal/tools/notices`、`scripts/check.sh` を通した PR)。
  - 依存が新しい Go を要求する場合 (`requires go >= 1.27.x`)、`go get` は `GOTOOLCHAIN=local` のコンテナでは失敗する。devcontainer のイメージを `--pull` で再ビルドし、`go version` を確認してから進める。`go.mod` の `go` は上がり、CI は `go-version-file: go.mod` なので追従する。
  - 先に別の Go モジュール PR をマージすると、残りの Dependabot PR は `go.mod` / `go.sum` が競合する (DIRTY)。閉じて、最新の main から同じ手順でやり直す。notices に差分が出ないこともある。
  - GUI はコンテナで確認できない。MyGo のような UI ライブラリが大きく上がるときは、マージ前に Release のドライラン + `scripts/fetch-build.sh` で Mac で見る。
- Dependabot security updates (脆弱性の修正 PR) は Settings で有効にしている。
