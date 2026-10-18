# pokedex-app

PokeAPI (https://pokeapi.co) を検索する、MyGo の native UI 製デスクトップアプリ。
macOS (arm64) 向けに配布する。ビルドとリリースは GitHub Actions が行う。

## 開発環境のルール
- 開発は devcontainer 上 (`scripts/dev.sh`)。非 root ユーザー `dev`。Go 1.27、gh、Claude Code が入っている。
- **ビルド成果物の配布用ビルドはローカルでやらない**。ビルドは GitHub Actions に任せる。ローカルで行うのは編集、`go vet`、`go test` まで。
- GUI はコンテナ内で表示できない。UI の確認は `ui.NewTester` によるテストと、CI が作った成果物を Mac で起動して行う。成果物は `scripts/fetch-build.sh` で `out-artifacts/` に取得する (ブラウザは使わない。手順は `docs/development.md`)。
- リポジトリは **Private**。Public 化の前のチェックリストは `docs/development.md`。シークレットや個人情報をコミットしない。
- 変えてはいけないもの: `mygo.json` の `identifier` (`com.github.naoya03708.pokedex`)。Go モジュール名 `github.com/naoya03708/pokedex-app`。
- MyGo の native UI の注意点は `.agents/skills/mygo-maintenance/SKILL.md` を読む。特に、要素を状態に保存しない、遅い I/O は UI スレッドの外で行い `Window.Update` で結果を反映する、の2点。
- MyGo の docs: https://github.com/egoist/mygo/tree/main/docs 、実装は `go list -m -f '{{.Dir}}' github.com/egoist/mygo` で読める。

## 要件と開発プロセス
- 要件: `docs/requirements.md` (FR-n と受入条件)。プロセス: `docs/development.md`。作業前に読む。
- 未完了の作業は GitHub Issues。このファイルにタスクや進捗を書かない。
- 受入条件駆動 + TDD: 要件 → 失敗するテスト (テスト名に FR ID) → 実装 → リファクタ。
- 機能ごとにブランチ + PR (Conventional Commits、英語)。Claude は PR 作成と CI 確認まで。マージ・タグ push・リポジトリ設定はユーザー。main に直接 push しない。
- PR 前に Definition of Done (`docs/development.md`) を満たす。`scripts/check.sh` は ci.yml の check ジョブをそのまま実行する。
- 機能の実装は skill `/feature FR-n` の手順で進める。PR を出す前に `reviewer` subagent で自己レビューする。
- hook が、編集ごとの fmt/vet、終了前の check.sh、危険な git/gh 操作の拒否を行う (詳細は `docs/development.md` の「AI ループ」)。hook に止められたら回避せず、示された理由に従う。
