# Security Policy / セキュリティポリシー

## Reporting a vulnerability / 脆弱性の報告

Please **do not open a public issue**. Use GitHub's private reporting:
**Security tab → Report a vulnerability**.

公開の Issue には書かず、GitHub の非公開の報告機能を使ってください
(リポジトリの **Security** タブ → **Report a vulnerability**)。

This is a personal project. I read reports on a best-effort basis and cannot
promise a response time.

個人のプロジェクトです。報告には可能な範囲で対応しますが、返信の期限は約束できません。

## Supported versions / 対象のバージョン

Only the [latest release](https://github.com/naoya03708/pokedex-app/releases/latest)
(macOS, Apple silicon). 最新のリリースのみが対象です。

## Scope / 対象の範囲

The app reads data from PokeAPI over HTTPS, keeps a cache under
`os.UserCacheDir()/pokedex`, and sends no telemetry.
Reports about how the app handles that data (for example, a crafted response
or a cache file) are in scope. The release is not signed with an Apple
Developer ID; the Gatekeeper warning described in the README is known.

アプリは PokeAPI のデータを HTTPS で取得し、`os.UserCacheDir()/pokedex` にキャッシュを保存します。
テレメトリは送りません。この取得データやキャッシュの扱いに関する報告は対象です。
Apple の Developer ID で署名していないことによる Gatekeeper の警告は、既知の事項です。
