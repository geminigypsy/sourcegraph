<!-- DO NOT EDIT: generated via: go generate ./enterprise/dev/ci -->

# Pipeline types reference

This is a reference outlining what CI pipelines we generate under different conditions.

To preview the pipeline for your branch, use `sg ci preview`.

For a higher-level overview, please refer to the [continuous integration docs](https://docs.sourcegraph.com/dev/background-information/continuous_integration).

## Run types

### Pull request

The default run type.

- Pipeline for `Go` changes:
  - **Linters and static analysis**: Prettier, Misc linters
  - **Go checks**: Test (all), Test (enterprise/internal/codeintel/stores/dbstore), Test (enterprise/internal/codeintel/stores/lsifstore), Test (enterprise/internal/insights), Test (internal/database), Test (internal/repos), Test (enterprise/internal/batches), Test (cmd/frontend), Test (enterprise/internal/database), Test (enterprise/cmd/frontend/internal/batches/resolvers), Build
  - Upload build trace

- Pipeline for `Client` changes:
  - **Pipeline setup**: Trigger async
  - **Linters and static analysis**: Prettier, Misc linters, Yarn deduplicate lint
  - **Client checks**: Puppeteer tests prep, Puppeteer tests chunk #1, Puppeteer tests chunk #2, Puppeteer tests chunk #3, Puppeteer tests chunk #4, Puppeteer tests chunk #5, Puppeteer tests chunk #6, Puppeteer tests chunk #7, Puppeteer tests chunk #8, Puppeteer tests chunk #9, Puppeteer tests finalize, Upload Storybook to Chromatic, Test shared client code, Test wildcard client code, Build, Enterprise build, Test, Puppeteer tests for chrome extension, Test browser extension, Test branded client code, Typescript eslint, Stylelint
  - Upload build trace

- Pipeline for `GraphQL` changes:
  - **Linters and static analysis**: Prettier, Misc linters, GraphQL lint
  - **Client checks**: Puppeteer tests prep, Puppeteer tests chunk #1, Puppeteer tests chunk #2, Puppeteer tests chunk #3, Puppeteer tests chunk #4, Puppeteer tests chunk #5, Puppeteer tests chunk #6, Puppeteer tests chunk #7, Puppeteer tests chunk #8, Puppeteer tests chunk #9, Puppeteer tests finalize, Upload Storybook to Chromatic, Test shared client code, Test wildcard client code, Build, Enterprise build, Test, Puppeteer tests for chrome extension, Test browser extension, Test branded client code, Typescript eslint, Stylelint
  - **Go checks**: Test (all), Test (enterprise/internal/codeintel/stores/dbstore), Test (enterprise/internal/codeintel/stores/lsifstore), Test (enterprise/internal/insights), Test (internal/database), Test (internal/repos), Test (enterprise/internal/batches), Test (cmd/frontend), Test (enterprise/internal/database), Test (enterprise/cmd/frontend/internal/batches/resolvers), Build
  - Upload build trace

- Pipeline for `DatabaseSchema` changes:
  - **Linters and static analysis**: Prettier, Misc linters
  - **DB backcompat tests**: Backcompat test (all), Backcompat test (enterprise/internal/codeintel/stores/dbstore), Backcompat test (enterprise/internal/codeintel/stores/lsifstore), Backcompat test (enterprise/internal/insights), Backcompat test (internal/database), Backcompat test (internal/repos), Backcompat test (enterprise/internal/batches), Backcompat test (cmd/frontend), Backcompat test (enterprise/internal/database), Backcompat test (enterprise/cmd/frontend/internal/batches/resolvers)
  - Upload build trace

- Pipeline for `Docs` changes:
  - **Linters and static analysis**: Prettier, Misc linters, Check and build docsite
  - Upload build trace

- Pipeline for `Dockerfiles` changes:
  - **Linters and static analysis**: Prettier, Misc linters, Docker linters
  - Upload build trace

- Pipeline for `ExecutorDockerRegistryMirror` changes:
  - **Linters and static analysis**: Prettier, Misc linters
  - Upload build trace

- Pipeline for `CIScripts` changes:
  - **Linters and static analysis**: Prettier, Misc linters
  - **CI script tests**: test-trace-command.sh
  - Upload build trace

- Pipeline for `Terraform` changes:
  - **Linters and static analysis**: Prettier, Misc linters, Checkov Terraform scanning
  - Upload build trace

- Pipeline for `SVG` changes:
  - **Linters and static analysis**: Prettier, Misc linters, SVG lint
  - Upload build trace

### Release branch nightly healthcheck build

The run type for environment including `{"RELEASE_NIGHTLY":"true"}`.

Default pipeline:

- Trigger 3.37 release branch healthcheck build
- Trigger 3.36 release branch healthcheck build
- Upload build trace

### Browser extension nightly release build

The run type for environment including `{"BEXT_NIGHTLY":"true"}`.

Default pipeline:

- Typescript eslint
- Stylelint
- Puppeteer tests for chrome extension
- Test browser extension
- Test shared client code
- Test wildcard client code
- E2E for chrome extension
- Upload build trace

### Tagged release

The run type for tags starting with `v`.

Default pipeline:

- **Pipeline setup**: Trigger async
- **Image builds**: Build alpine-3.12, Build alpine-3.14, Build cadvisor, Build codeinsights-db, Build codeintel-db, Build frontend, Build github-proxy, Build gitserver, Build grafana, Build indexed-searcher, Build jaeger-agent, Build jaeger-all-in-one, Build minio, Build postgres-12.6-alpine, Build postgres_exporter, Build precise-code-intel-worker, Build prometheus, Build redis-cache, Build redis-store, Build redis_exporter, Build repo-updater, Build search-indexer, Build searcher, Build symbols, Build syntax-highlighter, Build worker, Build migrator, Build server
- **Image security scans**: Scan alpine-3.12, Scan alpine-3.14, Scan cadvisor, Scan codeinsights-db, Scan codeintel-db, Scan frontend, Scan github-proxy, Scan gitserver, Scan grafana, Scan indexed-searcher, Scan jaeger-agent, Scan jaeger-all-in-one, Scan minio, Scan postgres-12.6-alpine, Scan postgres_exporter, Scan precise-code-intel-worker, Scan prometheus, Scan redis-cache, Scan redis-store, Scan redis_exporter, Scan repo-updater, Scan search-indexer, Scan searcher, Scan symbols, Scan syntax-highlighter, Scan worker, Scan migrator, Scan server
- **Linters and static analysis**: Prettier, Misc linters, GraphQL lint, SVG lint, Yarn deduplicate lint, Docker linters, Checkov Terraform scanning, Check and build docsite
- **Client checks**: Puppeteer tests prep, Puppeteer tests chunk #1, Puppeteer tests chunk #2, Puppeteer tests chunk #3, Puppeteer tests chunk #4, Puppeteer tests chunk #5, Puppeteer tests chunk #6, Puppeteer tests chunk #7, Puppeteer tests chunk #8, Puppeteer tests chunk #9, Puppeteer tests finalize, Upload Storybook to Chromatic, Test shared client code, Test wildcard client code, Build, Enterprise build, Test, Puppeteer tests for chrome extension, Test browser extension, Test branded client code, Typescript eslint, Stylelint
- **Go checks**: Test (all), Test (enterprise/internal/codeintel/stores/dbstore), Test (enterprise/internal/codeintel/stores/lsifstore), Test (enterprise/internal/insights), Test (internal/database), Test (internal/repos), Test (enterprise/internal/batches), Test (cmd/frontend), Test (enterprise/internal/database), Test (enterprise/cmd/frontend/internal/batches/resolvers), Build
- **DB backcompat tests**: Backcompat test (all), Backcompat test (enterprise/internal/codeintel/stores/dbstore), Backcompat test (enterprise/internal/codeintel/stores/lsifstore), Backcompat test (enterprise/internal/insights), Backcompat test (internal/database), Backcompat test (internal/repos), Backcompat test (enterprise/internal/batches), Backcompat test (cmd/frontend), Backcompat test (enterprise/internal/database), Backcompat test (enterprise/cmd/frontend/internal/batches/resolvers)
- **CI script tests**: test-trace-command.sh
- **Integration tests**: Backend integration tests, Code Intel QA
- **End-to-end tests**: Sourcegraph E2E, Sourcegraph QA, Sourcegraph Cluster (deploy-sourcegraph) QA, Sourcegraph Upgrade
- **Publish images**: alpine-3.12, alpine-3.14, cadvisor, codeinsights-db, codeintel-db, frontend, github-proxy, gitserver, grafana, indexed-searcher, jaeger-agent, jaeger-all-in-one, minio, postgres-12.6-alpine, postgres_exporter, precise-code-intel-worker, prometheus, redis-cache, redis-store, redis_exporter, repo-updater, search-indexer, searcher, symbols, syntax-highlighter, worker, migrator, server
- Upload build trace

### Release branch

The run type for branches matching `^[0-9]+\.[0-9]+$` (regexp match).

Default pipeline:

- **Pipeline setup**: Trigger async
- **Image builds**: Build alpine-3.12, Build alpine-3.14, Build cadvisor, Build codeinsights-db, Build codeintel-db, Build frontend, Build github-proxy, Build gitserver, Build grafana, Build indexed-searcher, Build jaeger-agent, Build jaeger-all-in-one, Build minio, Build postgres-12.6-alpine, Build postgres_exporter, Build precise-code-intel-worker, Build prometheus, Build redis-cache, Build redis-store, Build redis_exporter, Build repo-updater, Build search-indexer, Build searcher, Build symbols, Build syntax-highlighter, Build worker, Build migrator, Build server, Build executor image, Build docker registry mirror image
- **Image security scans**: Scan alpine-3.12, Scan alpine-3.14, Scan cadvisor, Scan codeinsights-db, Scan codeintel-db, Scan frontend, Scan github-proxy, Scan gitserver, Scan grafana, Scan indexed-searcher, Scan jaeger-agent, Scan jaeger-all-in-one, Scan minio, Scan postgres-12.6-alpine, Scan postgres_exporter, Scan precise-code-intel-worker, Scan prometheus, Scan redis-cache, Scan redis-store, Scan redis_exporter, Scan repo-updater, Scan search-indexer, Scan searcher, Scan symbols, Scan syntax-highlighter, Scan worker, Scan migrator, Scan server
- **Linters and static analysis**: Prettier, Misc linters, GraphQL lint, SVG lint, Yarn deduplicate lint, Docker linters, Checkov Terraform scanning, Check and build docsite
- **Client checks**: Puppeteer tests prep, Puppeteer tests chunk #1, Puppeteer tests chunk #2, Puppeteer tests chunk #3, Puppeteer tests chunk #4, Puppeteer tests chunk #5, Puppeteer tests chunk #6, Puppeteer tests chunk #7, Puppeteer tests chunk #8, Puppeteer tests chunk #9, Puppeteer tests finalize, Upload Storybook to Chromatic, Test shared client code, Test wildcard client code, Build, Enterprise build, Test, Puppeteer tests for chrome extension, Test browser extension, Test branded client code, Typescript eslint, Stylelint
- **Go checks**: Test (all), Test (enterprise/internal/codeintel/stores/dbstore), Test (enterprise/internal/codeintel/stores/lsifstore), Test (enterprise/internal/insights), Test (internal/database), Test (internal/repos), Test (enterprise/internal/batches), Test (cmd/frontend), Test (enterprise/internal/database), Test (enterprise/cmd/frontend/internal/batches/resolvers), Build
- **DB backcompat tests**: Backcompat test (all), Backcompat test (enterprise/internal/codeintel/stores/dbstore), Backcompat test (enterprise/internal/codeintel/stores/lsifstore), Backcompat test (enterprise/internal/insights), Backcompat test (internal/database), Backcompat test (internal/repos), Backcompat test (enterprise/internal/batches), Backcompat test (cmd/frontend), Backcompat test (enterprise/internal/database), Backcompat test (enterprise/cmd/frontend/internal/batches/resolvers)
- **CI script tests**: test-trace-command.sh
- **Integration tests**: Backend integration tests, Code Intel QA
- **End-to-end tests**: Sourcegraph E2E, Sourcegraph QA, Sourcegraph Cluster (deploy-sourcegraph) QA, Sourcegraph Upgrade
- **Publish images**: alpine-3.12, alpine-3.14, cadvisor, codeinsights-db, codeintel-db, frontend, github-proxy, gitserver, grafana, indexed-searcher, jaeger-agent, jaeger-all-in-one, minio, postgres-12.6-alpine, postgres_exporter, precise-code-intel-worker, prometheus, redis-cache, redis-store, redis_exporter, repo-updater, search-indexer, searcher, symbols, syntax-highlighter, worker, migrator, server, Publish executor image, Publish docker registry mirror image
- Upload build trace

### Browser extension release build

The run type for branches matching `bext/release` (exact match).

Default pipeline:

- Typescript eslint
- Stylelint
- Puppeteer tests for chrome extension
- Test browser extension
- Test shared client code
- Test wildcard client code
- E2E for chrome extension
- Extension release
- Extension release
- NPM Release
- Upload build trace

### Main branch

The run type for branches matching `main` (exact match).

Default pipeline:

- **Pipeline setup**: Trigger async
- **Image builds**: Build alpine-3.12, Build alpine-3.14, Build cadvisor, Build codeinsights-db, Build codeintel-db, Build frontend, Build github-proxy, Build gitserver, Build grafana, Build indexed-searcher, Build jaeger-agent, Build jaeger-all-in-one, Build minio, Build postgres-12.6-alpine, Build postgres_exporter, Build precise-code-intel-worker, Build prometheus, Build redis-cache, Build redis-store, Build redis_exporter, Build repo-updater, Build search-indexer, Build searcher, Build symbols, Build syntax-highlighter, Build worker, Build migrator, Build server, Build executor image, Build docker registry mirror image
- **Image security scans**: Scan alpine-3.12, Scan alpine-3.14, Scan cadvisor, Scan codeinsights-db, Scan codeintel-db, Scan frontend, Scan github-proxy, Scan gitserver, Scan grafana, Scan indexed-searcher, Scan jaeger-agent, Scan jaeger-all-in-one, Scan minio, Scan postgres-12.6-alpine, Scan postgres_exporter, Scan precise-code-intel-worker, Scan prometheus, Scan redis-cache, Scan redis-store, Scan redis_exporter, Scan repo-updater, Scan search-indexer, Scan searcher, Scan symbols, Scan syntax-highlighter, Scan worker, Scan migrator, Scan server
- **Linters and static analysis**: Prettier, Misc linters, GraphQL lint, SVG lint, Yarn deduplicate lint, Docker linters, Checkov Terraform scanning, Check and build docsite
- **Client checks**: Puppeteer tests prep, Puppeteer tests chunk #1, Puppeteer tests chunk #2, Puppeteer tests chunk #3, Puppeteer tests chunk #4, Puppeteer tests chunk #5, Puppeteer tests chunk #6, Puppeteer tests chunk #7, Puppeteer tests chunk #8, Puppeteer tests chunk #9, Puppeteer tests finalize, Upload Storybook to Chromatic, Test shared client code, Test wildcard client code, Build, Enterprise build, Test, Puppeteer tests for chrome extension, Test browser extension, Test branded client code, Typescript eslint, Stylelint
- **Go checks**: Test (all), Test (enterprise/internal/codeintel/stores/dbstore), Test (enterprise/internal/codeintel/stores/lsifstore), Test (enterprise/internal/insights), Test (internal/database), Test (internal/repos), Test (enterprise/internal/batches), Test (cmd/frontend), Test (enterprise/internal/database), Test (enterprise/cmd/frontend/internal/batches/resolvers), Build
- **DB backcompat tests**: Backcompat test (all), Backcompat test (enterprise/internal/codeintel/stores/dbstore), Backcompat test (enterprise/internal/codeintel/stores/lsifstore), Backcompat test (enterprise/internal/insights), Backcompat test (internal/database), Backcompat test (internal/repos), Backcompat test (enterprise/internal/batches), Backcompat test (cmd/frontend), Backcompat test (enterprise/internal/database), Backcompat test (enterprise/cmd/frontend/internal/batches/resolvers)
- **CI script tests**: test-trace-command.sh
- **Integration tests**: Backend integration tests, Code Intel QA
- **End-to-end tests**: Sourcegraph E2E, Sourcegraph QA, Sourcegraph Cluster (deploy-sourcegraph) QA, Sourcegraph Upgrade
- **Publish images**: alpine-3.12, alpine-3.14, cadvisor, codeinsights-db, codeintel-db, frontend, github-proxy, gitserver, grafana, indexed-searcher, jaeger-agent, jaeger-all-in-one, minio, postgres-12.6-alpine, postgres_exporter, precise-code-intel-worker, prometheus, redis-cache, redis-store, redis_exporter, repo-updater, search-indexer, searcher, symbols, syntax-highlighter, worker, migrator, server, Publish executor image, Publish docker registry mirror image
- Upload build trace

### Main dry run

The run type for branches matching `main-dry-run/`.
You can create a build of this run type for your changes using:

```sh
sg ci build main-dry-run
```

Default pipeline:

- **Pipeline setup**: Trigger async
- **Image builds**: Build alpine-3.12, Build alpine-3.14, Build cadvisor, Build codeinsights-db, Build codeintel-db, Build frontend, Build github-proxy, Build gitserver, Build grafana, Build indexed-searcher, Build jaeger-agent, Build jaeger-all-in-one, Build minio, Build postgres-12.6-alpine, Build postgres_exporter, Build precise-code-intel-worker, Build prometheus, Build redis-cache, Build redis-store, Build redis_exporter, Build repo-updater, Build search-indexer, Build searcher, Build symbols, Build syntax-highlighter, Build worker, Build migrator, Build server, Build executor image, Build docker registry mirror image
- **Image security scans**: Scan alpine-3.12, Scan alpine-3.14, Scan cadvisor, Scan codeinsights-db, Scan codeintel-db, Scan frontend, Scan github-proxy, Scan gitserver, Scan grafana, Scan indexed-searcher, Scan jaeger-agent, Scan jaeger-all-in-one, Scan minio, Scan postgres-12.6-alpine, Scan postgres_exporter, Scan precise-code-intel-worker, Scan prometheus, Scan redis-cache, Scan redis-store, Scan redis_exporter, Scan repo-updater, Scan search-indexer, Scan searcher, Scan symbols, Scan syntax-highlighter, Scan worker, Scan migrator, Scan server
- **Linters and static analysis**: Prettier, Misc linters, GraphQL lint, SVG lint, Yarn deduplicate lint, Docker linters, Checkov Terraform scanning, Check and build docsite
- **Client checks**: Puppeteer tests prep, Puppeteer tests chunk #1, Puppeteer tests chunk #2, Puppeteer tests chunk #3, Puppeteer tests chunk #4, Puppeteer tests chunk #5, Puppeteer tests chunk #6, Puppeteer tests chunk #7, Puppeteer tests chunk #8, Puppeteer tests chunk #9, Puppeteer tests finalize, Upload Storybook to Chromatic, Test shared client code, Test wildcard client code, Build, Enterprise build, Test, Puppeteer tests for chrome extension, Test browser extension, Test branded client code, Typescript eslint, Stylelint
- **Go checks**: Test (all), Test (enterprise/internal/codeintel/stores/dbstore), Test (enterprise/internal/codeintel/stores/lsifstore), Test (enterprise/internal/insights), Test (internal/database), Test (internal/repos), Test (enterprise/internal/batches), Test (cmd/frontend), Test (enterprise/internal/database), Test (enterprise/cmd/frontend/internal/batches/resolvers), Build
- **DB backcompat tests**: Backcompat test (all), Backcompat test (enterprise/internal/codeintel/stores/dbstore), Backcompat test (enterprise/internal/codeintel/stores/lsifstore), Backcompat test (enterprise/internal/insights), Backcompat test (internal/database), Backcompat test (internal/repos), Backcompat test (enterprise/internal/batches), Backcompat test (cmd/frontend), Backcompat test (enterprise/internal/database), Backcompat test (enterprise/cmd/frontend/internal/batches/resolvers)
- **CI script tests**: test-trace-command.sh
- **Integration tests**: Backend integration tests, Code Intel QA
- **End-to-end tests**: Sourcegraph E2E, Sourcegraph QA, Sourcegraph Cluster (deploy-sourcegraph) QA, Sourcegraph Upgrade
- **Publish images**: alpine-3.12, alpine-3.14, cadvisor, codeinsights-db, codeintel-db, frontend, github-proxy, gitserver, grafana, indexed-searcher, jaeger-agent, jaeger-all-in-one, minio, postgres-12.6-alpine, postgres_exporter, precise-code-intel-worker, prometheus, redis-cache, redis-store, redis_exporter, repo-updater, search-indexer, searcher, symbols, syntax-highlighter, worker, migrator, server
- Upload build trace

### Patch image

The run type for branches matching `docker-images-patch/`, requires a branch argument in the second branch path segment.
You can create a build of this run type for your changes using:

```sh
sg ci build docker-images-patch
```

### Patch image without testing

The run type for branches matching `docker-images-patch-notest/`, requires a branch argument in the second branch path segment.
You can create a build of this run type for your changes using:

```sh
sg ci build docker-images-patch-notest
```

### Build all candidates without testing

The run type for branches matching `docker-images-candidates-notest/`.
You can create a build of this run type for your changes using:

```sh
sg ci build docker-images-candidates-notest
```

Default pipeline:

- Build alpine-3.12
- Build alpine-3.14
- Build cadvisor
- Build codeinsights-db
- Build codeintel-db
- Build frontend
- Build github-proxy
- Build gitserver
- Build grafana
- Build indexed-searcher
- Build jaeger-agent
- Build jaeger-all-in-one
- Build minio
- Build postgres-12.6-alpine
- Build postgres_exporter
- Build precise-code-intel-worker
- Build prometheus
- Build redis-cache
- Build redis-store
- Build redis_exporter
- Build repo-updater
- Build search-indexer
- Build searcher
- Build symbols
- Build syntax-highlighter
- Build worker
- Build migrator
- Build server
- Upload build trace

### Build executor without testing

The run type for branches matching `executor-patch-notest/`.
You can create a build of this run type for your changes using:

```sh
sg ci build executor-patch-notest
```

Default pipeline:

- Build executor image
- Publish executor image
- Build docker registry mirror image
- Publish docker registry mirror image
- Upload build trace

### Backend integration tests

The run type for branches matching `backend-integration/`.
You can create a build of this run type for your changes using:

```sh
sg ci build backend-integration
```

Default pipeline:

- Build server
- Backend integration tests
- **Linters and static analysis**: Prettier, Misc linters, GraphQL lint, SVG lint, Yarn deduplicate lint, Docker linters, Checkov Terraform scanning, Check and build docsite
- **Client checks**: Puppeteer tests prep, Puppeteer tests chunk #1, Puppeteer tests chunk #2, Puppeteer tests chunk #3, Puppeteer tests chunk #4, Puppeteer tests chunk #5, Puppeteer tests chunk #6, Puppeteer tests chunk #7, Puppeteer tests chunk #8, Puppeteer tests chunk #9, Puppeteer tests finalize, Upload Storybook to Chromatic, Test shared client code, Test wildcard client code, Build, Enterprise build, Test, Puppeteer tests for chrome extension, Test browser extension, Test branded client code, Typescript eslint, Stylelint
- **Go checks**: Test (all), Test (enterprise/internal/codeintel/stores/dbstore), Test (enterprise/internal/codeintel/stores/lsifstore), Test (enterprise/internal/insights), Test (internal/database), Test (internal/repos), Test (enterprise/internal/batches), Test (cmd/frontend), Test (enterprise/internal/database), Test (enterprise/cmd/frontend/internal/batches/resolvers), Build
- **DB backcompat tests**: Backcompat test (all), Backcompat test (enterprise/internal/codeintel/stores/dbstore), Backcompat test (enterprise/internal/codeintel/stores/lsifstore), Backcompat test (enterprise/internal/insights), Backcompat test (internal/database), Backcompat test (internal/repos), Backcompat test (enterprise/internal/batches), Backcompat test (cmd/frontend), Backcompat test (enterprise/internal/database), Backcompat test (enterprise/cmd/frontend/internal/batches/resolvers)
- **CI script tests**: test-trace-command.sh
- Upload build trace
