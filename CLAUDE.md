# pokedex-app

PokeAPI (https://pokeapi.co) を検索する、MyGo の native UI 製デスクトップアプリ。
macOS (arm64) 向けに配布する (Windows は v0.1.0 では見送り)。ビルドとリリースは GitHub Actions が行う。

## 開発環境のルール
- 開発は devcontainer 上 (`scripts/dev.sh`)。非 root ユーザー `dev`。Go 1.27、gh、Claude Code が入っている。
- **ビルド成果物の配布用ビルドはローカルでやらない**。ビルドは GitHub Actions に任せる。ローカルで行うのは編集、`go vet`、`go test` まで。
- GUI はコンテナ内で表示できない。UI の確認は `ui.NewTester` によるテストと、CI が作った成果物を Mac で起動して行う。
- リポジトリは **Private** (v0.1.0 後に Public 化予定。チェックリストは `docs/development.md`)。シークレットや個人情報をコミットしない。
- MyGo の native UI の注意点は `.agents/skills/mygo-maintenance/SKILL.md` を読む。特に、要素を状態に保存しない、遅い I/O は UI スレッドの外で行い `Window.Update` で結果を反映する、の2点。
- MyGo の docs: https://github.com/egoist/mygo/tree/main/docs 、実装は `go list -m -f '{{.Dir}}' github.com/egoist/mygo` で読める。

## 要件と開発プロセス
- 要件: `docs/requirements.md` (FR-n と受入条件)。プロセス: `docs/development.md`。作業前に読む。
- 受入条件駆動 + TDD: 要件 → 失敗するテスト (テスト名に FR ID) → 実装 → リファクタ。
- 機能ごとにブランチ + PR (Conventional Commits、英語)。Claude は PR 作成と CI 確認まで。マージ・タグ push・リポジトリ設定はユーザー。main に直接 push しない。
- PR 前に Definition of Done (`docs/development.md`) を満たす。`scripts/check.sh` は ci.yml の check ジョブをそのまま実行する。
- 機能の実装は skill `/feature FR-n` の手順で進める。PR を出す前に `reviewer` subagent で自己レビューする。
- hook が、編集ごとの fmt/vet、終了前の check.sh、危険な git/gh 操作の拒否を行う (詳細は `docs/development.md` の「AI ループ」)。hook に止められたら回避せず、示された理由に従う。

## やること (上から順に。各項目は PR 単位で進める)

### 1. リポジトリ作成
- [x] `gh auth status` でログイン確認 (未ログインならユーザーに依頼)
- [x] `.gitignore` に `build/` と `out*` を含める
- [x] `go.mod` のモジュール名を `github.com/<owner>/pokedex-app` に変更 (`go mod edit -module`)
- [x] `mygo.json` の `identifier` を決める (例 `com.github.<owner>.pokedex`)。リリース後は変えない
- [x] git init → 初回コミット → `gh repo create pokedex-app --private --source . --push`

### 2. Claude Code 用の整備
- [x] `.agents/skills/mygo-maintenance` を `.claude/skills/` からも使えるようにする (symlink か複製)
- [x] 必要なら `.claude/settings.json` に `go test`、`go vet` などの許可を追加

### 3. PokeAPI クライアント (`internal/pokeapi`)
- [x] 名前一覧の取得 (`/pokemon?limit=100000`)。PokeAPI に検索エンドポイントはないので、一覧をディスクにキャッシュして**クライアント側で絞り込む**
- [x] 詳細取得 (`/pokemon/{name}`: タイプ、種族値、スプライト)
- [x] 日本語名 (`/pokemon-species/{id}` の `names`) を扱うか決める → v0.1 では扱わない
- [x] `http.Client` にタイムアウト、`context` 対応、キャッシュ (Fair use のため同じ取得を繰り返さない)
- [x] `httptest` で外部通信なしのテストを書く

### 4. UI (native)
- [x] 検索ボックスと結果リスト、詳細ペイン (画像、タイプ、種族値)
- [x] 読み込み中・エラー・0件の表示
- [x] ネットワーク取得は goroutine で行い、`Window.Update` で反映。検索語が変わったら古い結果を捨てる
- [x] `ui.NewTester` で操作テスト

### 5. CI (`.github/workflows/ci.yml`)
- [x] push と PR で `gofmt -l`、`go vet`、`go tool mygo vet .`、`go test -race -cover` (カバレッジを Job Summary に出す)
- [x] ubuntu のみで実行 (macOS runner はリリース時だけ)
- [x] アクションはコミット SHA で固定、`permissions: contents: read`
- [ ] **2026-10-26 以降**: `go get -tool honnef.co/go/tools/cmd/staticcheck@v0.8.2` で staticcheck を追加し、CI と DoD に入れる (v0.8.1 以前は Go 1.27 非対応。v0.8.2 は 2026-10-12 公開のため 2 週間待つ)

### 6. リリース (`.github/workflows/release.yml`)
- [x] `v*` タグの push で起動、`permissions: contents: write`
- [x] macOS: `macos-latest` で `go tool mygo build -platform darwin/arm64`、アドホック署名、dmg
- [x] ~~Windows: ビルド方法を決める~~ v0.1.0 では見送り (2026-10-13 に決定。要件の範囲外に移した)
- [x] 成果物を GitHub Release に添付 (`gh release create` / `gh release upload`)
- [ ] Private リポジトリでは Actions の無料枠に分数制限があり、macOS runner は消費が大きい点に注意

### 7. 動作確認と公開
- [ ] `v0.1.0` タグでリリースし、成果物を Mac で起動して確認
- [x] 未署名なので初回起動は Gatekeeper の警告が出る。手順を README に書く
- [x] README (使い方、ビルド、リリース手順) を書く (スクリーンショットは Public 化前のチェックリストで追加)

## 決定事項
- オーナー: `naoya03708`。モジュール `github.com/naoya03708/pokedex-app`、identifier `com.github.naoya03708.pokedex` (変更しない)
- Windows: v0.1.0 では見送り (2026-10-13)。再開するときは exe の zip を想定
- macOS: `arm64` のみ
- 日本語名検索: 当面は対応しない (英語名のみ。後で追加を検討)
