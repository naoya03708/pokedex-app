# Pokedex

[PokeAPI](https://pokeapi.co) を引く、macOS 向けのデスクトップ図鑑アプリです。名前で絞り込み、タイプ・種族値・スプライトを表示します。UI は Go 製の [MyGo](https://github.com/egoist/mygo) の native UI で、Web ビューは使っていません。

このリポジトリは、**要件 → テスト → 実装 → CI → 配布**という一連の流れを示すポートフォリオでもあります。開発の進め方は [docs/development.md](docs/development.md)、要件は [docs/requirements.md](docs/requirements.md) にあります。

## 機能
- **名前で検索**: 入力するたびにその場で絞り込みます (大文字小文字は区別しません)。検索ボタンはありません。
- **詳細表示**: スプライト、名前、図鑑番号 (`#025`)、タイプ、種族値 6 項目を数値とバーで表示します。
- **キーボード操作**: `Tab` でリストに移り、`↑` `↓` で行を移動し、`Enter` で選びます。
- **状態の表示**: 読み込み中、エラー (Retry ボタン付き)、0 件 (`No Pokémon found`)。
- **キャッシュ**: 一度取得したものはディスクに保存します。キャッシュにあるものはオフラインでも表示できます。

UI は英語です。日本語名での検索は、今のところ対応していません。

## インストール (macOS)
対応は **macOS (Apple silicon / arm64)** のみです。Windows と Intel Mac は、今のところ対応していません。

1. [Releases](https://github.com/naoya03708/pokedex-app/releases) から `Pokedex-X.Y.Z-macos-arm64.dmg` をダウンロードします。

   Chrome などのブラウザは、署名のない dmg を「安全でないファイル」としてブロックすることがあります。その場合は、ダウンロードの一覧から「保存」(Keep) を選ぶか、[GitHub CLI](https://cli.github.com/) で取得します。CLI で保存したファイルには macOS の隔離属性が付かないので、下の初回起動の警告も出にくくなります。

   ```sh
   gh release download vX.Y.Z --repo naoya03708/pokedex-app --pattern '*.dmg' --pattern 'SHA256SUMS.txt'
   ```

2. (任意) 同じ場所の `SHA256SUMS.txt` で、ファイルが壊れていないか確かめます。

   ```sh
   cd ~/Downloads && shasum -a 256 -c SHA256SUMS.txt
   ```

3. dmg を開き、`Pokedex.app` を `Applications` にコピーします。

### 初回起動の警告について
このアプリは Apple の Developer ID による署名と公証 (notarization) を受けていません (アドホック署名です)。そのため、初回起動で次の警告が出ます。

> Appleは、“Pokedex.app”にMacに損害を与えたり、プライバシーを侵害する可能性のあるマルウェアが含まれていないことを検証できませんでした。

マルウェアが見つかったという意味ではなく、Apple が検証していないという意味です。次の手順で開けます。

1. 警告ダイアログは「完了」で閉じます。
2. 「システム設定」→「プライバシーとセキュリティ」を開きます。
3. 下の方に出ている「"Pokedex.app"はブロックされました」の「このまま開く」を押し、パスワードを入力します。

最近の macOS では、Finder の右クリック →「開く」では回避できません。ターミナルを使う場合は、次のコマンドで隔離属性を外しても開けます。

```sh
xattr -dr com.apple.quarantine /Applications/Pokedex.app
```

## データとプライバシー
- テレメトリは送りません。通信先は PokeAPI (`pokeapi.co`) と、API が示すスプライト画像の置き場所 (PokeAPI のスプライトリポジトリ) だけです。
- 取得した内容は `os.UserCacheDir()/pokedex` に保存します。macOS では `~/Library/Caches/pokedex` です。名前の一覧は 7 日で再取得し、詳細とスプライトは期限なしで保存します。消したいときはこのフォルダを削除してください。
- [PokeAPI の Fair use ポリシー](https://pokeapi.co/docs/v2)に従い、同じ内容を繰り返し取得しないようにキャッシュしています。

ポケモンに関する名称・画像の権利は、各権利者に帰属します。このアプリは非公式で、権利者とは関係ありません。

## 開発
開発は devcontainer (`scripts/dev.sh`) の中で行います。

```sh
go test -race ./...   # テスト (外部通信はしない)
scripts/check.sh      # CI の check ジョブと同じ内容をローカルで実行する
```

- GUI はコンテナ内で表示できません。UI は `ui.NewTester` によるテストで確かめ、実際の見た目は CI が作った成果物を Mac で起動して確かめます。
- 配布用のビルドはローカルでは行わず、GitHub Actions に任せます。
- 開発の進め方、ブランチと PR の運用、リリース手順は [docs/development.md](docs/development.md) にあります。

### 構成
依存は一方向です: `main` → `internal/ui` → `internal/app` → `internal/pokeapi`

| パッケージ | 役割 |
|---|---|
| `internal/pokeapi` | PokeAPI のクライアント。タイムアウト、`context` 対応、メモリとディスクのキャッシュ |
| `internal/app` | UI に依存しない状態 (検索語、絞り込み、選択、取得状態)。取得は goroutine で行い、古い結果を捨てる |
| `internal/ui` | `app.Model` から画面を作る MyGo の view |

## ライセンス
[MIT License](LICENSE) です。

アプリには、依存ライブラリ (MyGo など) のライセンス表示を `THIRD_PARTY_NOTICES.txt` として同梱しています (`Pokedex.app/Contents/Resources/` の中)。このファイルは `go run ./internal/tools/notices` で生成し、依存が変わると CI のテストが古さを検出します。
