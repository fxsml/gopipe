# Release Session Plan

This document outlines the plan for setting up git flow and preparing the next release.

## Session Overview

**Objective**: Set up git flow, integrate CI/quality gates, and prepare feature branches for the next release.

**Status**: In Progress

## Phase 1: Infrastructure Setup

### 1.1 Integrate CI/Makefile/Badges Branch
- Source: `origin/claude/add-ci-makefile-badges-018M26mSpqaLvXdrhcx4q8HD`
- Cherry-pick commit `36ada7c` (CI pipeline, Makefile, badges)
- Reduce Makefile targets to essential set
- Add `git-semver` tool for release management
- Run quality gates (skip lint if too many errors)

### 1.2 Initialize Git Flow
- Create `develop` branch from `main`
- Set up branch naming conventions:
  - `feature/*` - new features
  - `release/*` - release preparation
  - `hotfix/*` - critical fixes
  - `bugfix/*` - non-critical fixes

## Phase 2: Feature Integration

### 2.1 Feature Branches (in dependency order)

| # | Branch | Commit | Description |
|---|--------|--------|-------------|
| 1 | feature-01-channel-groupby | d09a423 | Key-based batching |
| 2 | feature-02-message-core | e4e3efd | Core Message refactoring |
| 3 | feature-03-message-pubsub | 753b933 | Pub/Sub + CloudEvents |
| 4 | feature-04-message-router | f17e6e5 | Router with handlers/matchers |
| 5 | feature-05-message-cqrs | 2b5d8fb | CQRS command/event handlers |
| 6 | feature-07-multiplex | abe3852 | Topic-based routing |
| 7 | feature-08-middleware | e7ae7a0 | Reusable middleware components |

### 2.2 Documentation Branch
- Source: `origin/claude/document-pubsub-features-0179VA8zcKcjHsub6oFAGcRL`
- Includes: CHANGELOG, CONTRIBUTING, feature docs, ADRs

## Phase 3: GitHub Integration

**Note**: GitHub CLI not authenticated. Issues and PRs must be created manually.

### 3.1 Issues to Create

| # | Title | Description |
|---|-------|-------------|
| 1 | feat(channel): add GroupBy for key-based batching | Implement GroupBy function for partitioning messages by key |
| 2 | feat(message): refactor core Message type | Simplify Message type and align with CloudEvents |
| 3 | feat(message): add Publisher/Subscriber and Broker | Implement pub/sub pattern with CloudEvents support |
| 4 | feat(message): add Router with handlers/matchers | Message routing with middleware support |
| 5 | feat(message/cqrs): add CQRS handlers | Type-safe command and event handlers |
| 6 | feat(message/multiplex): topic-based routing | Topic-based routing for Sender/Receiver |
| 7 | feat(middleware): reusable middleware components | Correlation and message middleware |
| 8 | docs: comprehensive documentation | CHANGELOG, CONTRIBUTING, feature docs, ADRs |

### 3.2 Pull Requests to Create
- Feature branches → develop (one PR per feature)
- develop → release/vX.Y.Z
- release/vX.Y.Z → main

## Phase 4: Release Preparation

### 4.1 Create Release Branch
- Branch: `release/v0.x.0` (version TBD based on semver)
- Include all features merged to develop
- Final testing and quality gates

### 4.2 Cleanup
- Remove this plan file before merging to main
- Update CHANGELOG with release date
- Tag release with git-semver

## Notes

- GitHub CLI (`gh`) not available - issues/PRs may need manual creation
- Lint errors expected - will be addressed separately if extensive

## Progress Tracking

- [x] Phase 1: Infrastructure Setup
  - [x] Integrate CI/Makefile
  - [x] Add git-semver
  - [x] Initialize git flow (develop branch created)
- [x] Phase 2: Feature Integration
  - [x] Merge feature branches to develop
  - [x] Merge documentation branch to develop
- [x] Phase 3: GitHub Integration
  - [x] Create issues (#35-#42)
  - [x] Create release PR (#43)
- [ ] Phase 4: Release Preparation
  - [x] Create release branch (release/v0.10.0)
  - [x] Final testing (all tests pass)
  - [ ] Merge to main (USER ACTION REQUIRED)
  - [ ] Tag release v0.10.0
  - [ ] Merge release back to develop

## GitHub Issues Created

All issues created and linked to the release:

| # | Issue | Title |
|---|-------|-------|
| 1 | [#35](https://github.com/fxsml/gopipe/issues/35) | feat(channel): add GroupBy for key-based batching |
| 2 | [#36](https://github.com/fxsml/gopipe/issues/36) | feat(message): refactor core Message type |
| 3 | [#37](https://github.com/fxsml/gopipe/issues/37) | feat(message): add Publisher/Subscriber and Broker |
| 4 | [#38](https://github.com/fxsml/gopipe/issues/38) | feat(message): add Router with handlers/matchers |
| 5 | [#39](https://github.com/fxsml/gopipe/issues/39) | feat(message/cqrs): add CQRS handlers |
| 6 | [#40](https://github.com/fxsml/gopipe/issues/40) | feat(message/multiplex): topic-based routing |
| 7 | [#41](https://github.com/fxsml/gopipe/issues/41) | feat(middleware): reusable middleware components |
| 8 | [#42](https://github.com/fxsml/gopipe/issues/42) | docs: comprehensive documentation |

## Pull Request

**Release PR**: [#43](https://github.com/fxsml/gopipe/pull/43) - release: v0.10.0 - Major pub/sub implementation

## Remaining Steps

1. **Review and merge PR #43** to main
2. **After merge**, tag the release:
   ```bash
   git checkout main
   git pull
   git tag v0.10.0
   git push origin v0.10.0
   ```
3. **Merge release back to develop**:
   ```bash
   git checkout develop
   git merge main
   git push origin develop
   ```
4. **Delete this plan file** (optional - can keep for reference)
