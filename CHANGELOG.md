# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- PokeAPI client (`internal/pokeapi`): name list and Pokémon detail with 10 s timeout, `context` cancellation, memory and disk cache (list expires after 7 days, details never; stale list is served when offline).
- App model (`internal/app`): case-insensitive name filtering, loading/error states with retry, and rejection and cancellation of stale detail fetches.
- Sprite images: fetched as PNG bytes, cached in memory and on disk without expiry, and loaded after the detail text so a failed image never hides the detail.
- Native window UI (`internal/ui`): search box, result list with keyboard navigation (Up/Down move, Enter selects), detail pane with sprite, types and base-stat bars, and loading, error (with Retry) and "No Pokémon found" states.
