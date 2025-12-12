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

### 3.1 Create Issues
- Create GitHub issues for each feature
- Use issue references in commits (See #N, Closes #N)

### 3.2 Create Pull Requests
- Feature branches → develop
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

- [ ] Phase 1: Infrastructure Setup
  - [ ] Integrate CI/Makefile
  - [ ] Add git-semver
  - [ ] Initialize git flow
- [ ] Phase 2: Feature Integration
  - [ ] Create feature branches
  - [ ] Cherry-pick feature commits
- [ ] Phase 3: GitHub Integration
  - [ ] Create issues
  - [ ] Create PRs
- [ ] Phase 4: Release Preparation
  - [ ] Create release branch
  - [ ] Final testing
  - [ ] Cleanup and merge
