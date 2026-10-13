# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- PokeAPI client (`internal/pokeapi`): name list and Pokémon detail with 10 s timeout, `context` cancellation, memory and disk cache (list expires after 7 days, details never; stale list is served when offline).
