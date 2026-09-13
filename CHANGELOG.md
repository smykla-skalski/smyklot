# Changelog

## [1.55.0](https://github.com/smykla-skalski/smyklot/compare/v1.54.2...v1.55.0) (2026-09-13)

### Features

* **formatting:** cap inline collection length ([#386](https://github.com/smykla-skalski/smyklot/issues/386)) ([b5b0d40](https://github.com/smykla-skalski/smyklot/commit/b5b0d40da5502a8026f10edc5199f199ef4d9970))

### Bug Fixes

* **deps:** update module golang.org/x/oauth2 to v0.37.0 ([#374](https://github.com/smykla-skalski/smyklot/issues/374)) ([96c4599](https://github.com/smykla-skalski/smyklot/commit/96c4599245f87ee74b371e27dc4c9deaec82964a))

## [1.54.2](https://github.com/smykla-skalski/smyklot/compare/v1.54.1...v1.54.2) (2026-09-11)

### Bug Fixes

* **deps:** update ginkgo to v2.32.2 ([#376](https://github.com/smykla-skalski/smyklot/issues/376)) ([21cb35f](https://github.com/smykla-skalski/smyklot/commit/21cb35f138b05cf542fe99df4038892390c75f94))
* **deps:** update module github.com/jackc/pgx/v5 to v5.11.0 ([#373](https://github.com/smykla-skalski/smyklot/issues/373)) ([9aa474a](https://github.com/smykla-skalski/smyklot/commit/9aa474ad656bfa562abfff4688bf88a373e4b779))
* **deps:** update module github.com/yuin/goldmark/v2 to v2.0.2 ([#380](https://github.com/smykla-skalski/smyklot/issues/380)) ([aeb1718](https://github.com/smykla-skalski/smyklot/commit/aeb17189bf94e5cc59d219afc58e6b55178b7417))
* **deps:** update module golang.org/x/sync to v0.23.0 ([#375](https://github.com/smykla-skalski/smyklot/issues/375)) ([cd18e40](https://github.com/smykla-skalski/smyklot/commit/cd18e40c05a65a2ad15689ba95708524ac982e0a))

## [1.54.1](https://github.com/smykla-skalski/smyklot/compare/v1.54.0...v1.54.1) (2026-09-07)

### Bug Fixes

* **panel:** preserve edits and stabilize settings controls ([#370](https://github.com/smykla-skalski/smyklot/issues/370)) ([f0c9ba1](https://github.com/smykla-skalski/smyklot/commit/f0c9ba17f211fbea67e3b9b3cfab5def5a6ad79b))

## [1.54.0](https://github.com/smykla-skalski/smyklot/compare/v1.53.0...v1.54.0) (2026-09-07)

### Features

* **panel:** make sync settings easier to manage ([#366](https://github.com/smykla-skalski/smyklot/issues/366)) ([4217cf1](https://github.com/smykla-skalski/smyklot/commit/4217cf19cb78207b07305fc190b2c70802deb22f))

### Bug Fixes

* **deps:** update dependency @codemirror/legacy-modes to v6.5.4 ([#365](https://github.com/smykla-skalski/smyklot/issues/365)) ([5a1008e](https://github.com/smykla-skalski/smyklot/commit/5a1008ecce88f0137db2719fb0e54e42e1231699))
* **storage:** write a copied timestamp readably ([#363](https://github.com/smykla-skalski/smyklot/issues/363)) ([ffd49c9](https://github.com/smykla-skalski/smyklot/commit/ffd49c9092ec72c5dd7cabe75b94b3b93c6a96b4))

## [1.53.0](https://github.com/smykla-skalski/smyklot/compare/v1.52.0...v1.53.0) (2026-09-06)

### Features

* **sync:** run routine sync automatically ([#364](https://github.com/smykla-skalski/smyklot/issues/364)) ([5e1d9c9](https://github.com/smykla-skalski/smyklot/commit/5e1d9c9b35cc1633e19f03399a6ee74ff7c0c0ea))

### Bug Fixes

* **service:** measure before waiting five minutes ([#361](https://github.com/smykla-skalski/smyklot/issues/361)) ([bc53bb8](https://github.com/smykla-skalski/smyklot/commit/bc53bb8533e553c300f90b4ca03cad415c8719c3))

## [1.52.0](https://github.com/smykla-skalski/smyklot/compare/v1.51.0...v1.52.0) (2026-09-05)

### Features

* **panel:** chart what the service has cost itself ([#358](https://github.com/smykla-skalski/smyklot/issues/358)) ([225a3b7](https://github.com/smykla-skalski/smyklot/commit/225a3b7fe43b8fd206161499d797200df29184c3))

### Bug Fixes

* **deps:** update module github.com/google/go-github/v90 to v91 ([#352](https://github.com/smykla-skalski/smyklot/issues/352)) ([5076d57](https://github.com/smykla-skalski/smyklot/commit/5076d575f7b3bee93dc5939738cc72f612ffe04d))
* **deps:** update module github.com/yuin/goldmark/v2 to v2.0.1 ([#353](https://github.com/smykla-skalski/smyklot/issues/353)) ([3ccc766](https://github.com/smykla-skalski/smyklot/commit/3ccc766b1b66c47ff8a41b7db9eb5dbf42001c3b))
* **queue:** unblock retries and bound the ledger ([#357](https://github.com/smykla-skalski/smyklot/issues/357)) ([5ec924a](https://github.com/smykla-skalski/smyklot/commit/5ec924a328e9adadc30620ddfea7ed09d59b2a5a))

## [1.51.0](https://github.com/smykla-skalski/smyklot/compare/v1.50.4...v1.51.0) (2026-09-02)

### Features

* **panel:** rebuild the sync plan on typed actions ([#350](https://github.com/smykla-skalski/smyklot/issues/350)) ([65c675f](https://github.com/smykla-skalski/smyklot/commit/65c675f4bb9dfa9c4e1d6637a902522fdee3a40d)), closes [#color](https://github.com/smykla-skalski/smyklot/issues/color) [#0e8a16](https://github.com/smykla-skalski/smyklot/issues/0e8a16)

### Bug Fixes

* **deps:** update module modernc.org/sqlite to v1.58.0 ([#349](https://github.com/smykla-skalski/smyklot/issues/349)) ([abf19ac](https://github.com/smykla-skalski/smyklot/commit/abf19ac861e70da8e6afa3c1654ea3e003294223))

## [1.50.4](https://github.com/smykla-skalski/smyklot/compare/v1.50.3...v1.50.4) (2026-09-02)

### Bug Fixes

* **panel:** stop the sync plan crashing on a profile timezone ([#348](https://github.com/smykla-skalski/smyklot/issues/348)) ([651641f](https://github.com/smykla-skalski/smyklot/commit/651641f3f2eee294371b2dcd4ca91390ba4be85e))

## [1.50.3](https://github.com/smykla-skalski/smyklot/compare/v1.50.2...v1.50.3) (2026-09-02)

### Bug Fixes

* **deps:** update dependency @tanstack/svelte-table to v9.2.4 ([#343](https://github.com/smykla-skalski/smyklot/issues/343)) ([1626f91](https://github.com/smykla-skalski/smyklot/commit/1626f9127e567ec9c7f4bd4b026ad86d7ccde28f))

## [1.50.2](https://github.com/smykla-skalski/smyklot/compare/v1.50.1...v1.50.2) (2026-09-02)

### Code Refactoring

* **panel:** draw the panel from the design system it was built to ([#347](https://github.com/smykla-skalski/smyklot/issues/347)) ([0bfd4d3](https://github.com/smykla-skalski/smyklot/commit/0bfd4d39f89cca663cd202623328e4ca12e9ce5b))

## [1.50.1](https://github.com/smykla-skalski/smyklot/compare/v1.50.0...v1.50.1) (2026-08-31)

### Bug Fixes

* **deps:** update dependency @tanstack/svelte-table to v9.2.3 ([#340](https://github.com/smykla-skalski/smyklot/issues/340)) ([27acf17](https://github.com/smykla-skalski/smyklot/commit/27acf1778f0fd7bb54771b2fe0a72fb885806145))

## [1.50.0](https://github.com/smykla-skalski/smyklot/compare/v1.49.1...v1.50.0) (2026-08-28)

### Features

* **sync:** configure preserve-first file formatting ([#333](https://github.com/smykla-skalski/smyklot/issues/333)) ([c88a4cc](https://github.com/smykla-skalski/smyklot/commit/c88a4cc5cd33a492988809141defb4998679657f))

### Bug Fixes

* **deps:** update github.com/tailscale/hujson digest to b80ff77 ([#334](https://github.com/smykla-skalski/smyklot/issues/334)) ([f21349c](https://github.com/smykla-skalski/smyklot/commit/f21349cb7c6b7a5d11b2886d33040cdb57c1a521))
* **panel:** stabilize frontend release validation ([#336](https://github.com/smykla-skalski/smyklot/issues/336)) ([bb2ab68](https://github.com/smykla-skalski/smyklot/commit/bb2ab68332b7bd1118ce71524fa61464c373cffc))

## [1.49.1](https://github.com/smykla-skalski/smyklot/compare/v1.49.0...v1.49.1) (2026-08-28)

### Bug Fixes

* **deps:** update module github.com/onsi/gomega to v1.43.0 ([#330](https://github.com/smykla-skalski/smyklot/issues/330)) ([e7b68e7](https://github.com/smykla-skalski/smyklot/commit/e7b68e794af1280ce45792c0577e1bd59eec95a6))

## [1.49.0](https://github.com/smykla-skalski/smyklot/compare/v1.48.4...v1.49.0) (2026-08-25)

### Features

* **merge:** allow merging draft pull requests ([#321](https://github.com/smykla-skalski/smyklot/issues/321)) ([f4df619](https://github.com/smykla-skalski/smyklot/commit/f4df6199d0dc90b5194c6a6dc9e4cfa456b008d7))

### Bug Fixes

* **queue:** stop disabled repository work ([#326](https://github.com/smykla-skalski/smyklot/issues/326)) ([b9d29b5](https://github.com/smykla-skalski/smyklot/commit/b9d29b5d57bbdfa5a289111bfbc8e8e0ff8d7ee5))

## [1.48.4](https://github.com/smykla-skalski/smyklot/compare/v1.48.3...v1.48.4) (2026-08-25)

### Bug Fixes

* **service:** own listeners before serving ([#325](https://github.com/smykla-skalski/smyklot/issues/325)) ([25b06cc](https://github.com/smykla-skalski/smyklot/commit/25b06ccf95d070a7e880b1b466d93ded4e177c7d))

## [1.48.3](https://github.com/smykla-skalski/smyklot/compare/v1.48.2...v1.48.3) (2026-08-25)

### Performance Improvements

* **queue:** bound retention cleanup ([#324](https://github.com/smykla-skalski/smyklot/issues/324)) ([6d2c796](https://github.com/smykla-skalski/smyklot/commit/6d2c7960672b490d4d8ce661349069ed799aba97))

## [1.48.2](https://github.com/smykla-skalski/smyklot/compare/v1.48.1...v1.48.2) (2026-08-25)

### Bug Fixes

* **queue:** bound production dispatch work ([#320](https://github.com/smykla-skalski/smyklot/issues/320)) ([79bd262](https://github.com/smykla-skalski/smyklot/commit/79bd262c65df55fa424d0926cbdc9b6ff1d76e0a))
* **queue:** keep durable work moving and visible ([#322](https://github.com/smykla-skalski/smyklot/issues/322)) ([b03b126](https://github.com/smykla-skalski/smyklot/commit/b03b1264a6e299a9d7ff6065fc09e22d9a4be1ea))

## [1.48.1](https://github.com/smykla-skalski/smyklot/compare/v1.48.0...v1.48.1) (2026-08-25)

### Bug Fixes

* **queue:** restore production scheduling ([#319](https://github.com/smykla-skalski/smyklot/issues/319)) ([01274e8](https://github.com/smykla-skalski/smyklot/commit/01274e8fc094e5fe8666c5a1ef78e3aa16dffb7a))

## [1.48.0](https://github.com/smykla-skalski/smyklot/compare/v1.47.1...v1.48.0) (2026-08-25)

### Features

* **queue:** schedule all background work ([#317](https://github.com/smykla-skalski/smyklot/issues/317)) ([4b9d93c](https://github.com/smykla-skalski/smyklot/commit/4b9d93cedf355b4791d765021f9682c3f48d7e0e))

## [1.47.1](https://github.com/smykla-skalski/smyklot/compare/v1.47.0...v1.47.1) (2026-08-24)

### Bug Fixes

* **panel:** harden settings draft workflow ([#314](https://github.com/smykla-skalski/smyklot/issues/314)) ([382c646](https://github.com/smykla-skalski/smyklot/commit/382c6466f19f09c15d2a56dcdfb009e85be6cd5d))

## [1.47.0](https://github.com/smykla-skalski/smyklot/compare/v1.46.2...v1.47.0) (2026-08-23)

### Features

* **panel:** unify workspace setting drafts ([#312](https://github.com/smykla-skalski/smyklot/issues/312)) ([4013c5a](https://github.com/smykla-skalski/smyklot/commit/4013c5a6bb7f28b6a0fe849ad08930ced964ccbc))

## [1.46.2](https://github.com/smykla-skalski/smyklot/compare/v1.46.1...v1.46.2) (2026-08-23)

### Bug Fixes

* **panel:** contain long Sync labels ([#311](https://github.com/smykla-skalski/smyklot/issues/311)) ([4feb1d4](https://github.com/smykla-skalski/smyklot/commit/4feb1d4d380ba7aa513fa1c2b3b938f95aee01cd))

## [1.46.1](https://github.com/smykla-skalski/smyklot/compare/v1.46.0...v1.46.1) (2026-08-23)

### Bug Fixes

* **panel:** restore section navigation ([#310](https://github.com/smykla-skalski/smyklot/issues/310)) ([8c908bf](https://github.com/smykla-skalski/smyklot/commit/8c908bf915f5285bc12dda7cee939e7b4e4ae7e2))

## [1.46.0](https://github.com/smykla-skalski/smyklot/compare/v1.45.6...v1.46.0) (2026-08-23)

### Features

* **sync:** add save history and restore ([#308](https://github.com/smykla-skalski/smyklot/issues/308)) ([6f6a11b](https://github.com/smykla-skalski/smyklot/commit/6f6a11bb5673696f995d3d30322ea67e670609a8))

## [1.45.6](https://github.com/smykla-skalski/smyklot/compare/v1.45.5...v1.45.6) (2026-08-22)

### Bug Fixes

* **panel:** repair production status boundaries ([#304](https://github.com/smykla-skalski/smyklot/issues/304)) ([34b02d7](https://github.com/smykla-skalski/smyklot/commit/34b02d7deec420c766c7d102942cc239c4770aa3))

## [1.45.5](https://github.com/smykla-skalski/smyklot/compare/v1.45.4...v1.45.5) (2026-08-22)

### Bug Fixes

* **panel:** show sync editor logins ([#303](https://github.com/smykla-skalski/smyklot/issues/303)) ([8686a54](https://github.com/smykla-skalski/smyklot/commit/8686a54c11249b1878cbc01d4b7382caa2c57d0d))

## [1.45.4](https://github.com/smykla-skalski/smyklot/compare/v1.45.3...v1.45.4) (2026-08-22)

### Code Refactoring

* **github:** take this bot's vocabulary out of the client ([#302](https://github.com/smykla-skalski/smyklot/issues/302)) ([cf56195](https://github.com/smykla-skalski/smyklot/commit/cf56195bb57bd05b980f5f4323ca6211de828f81))

## [1.45.3](https://github.com/smykla-skalski/smyklot/compare/v1.45.2...v1.45.3) (2026-08-22)

### Bug Fixes

* **panel:** repair broken UI states ([#300](https://github.com/smykla-skalski/smyklot/issues/300)) ([6df36aa](https://github.com/smykla-skalski/smyklot/commit/6df36aa20e54f0b23acc5229fce42931bbbdae35))

### Code Refactoring

* **webhook:** make the delivery pipeline reusable ([#299](https://github.com/smykla-skalski/smyklot/issues/299)) ([7280d4c](https://github.com/smykla-skalski/smyklot/commit/7280d4c1ac1a2162dd60ec03b836e65367f0c850))

## [1.45.2](https://github.com/smykla-skalski/smyklot/compare/v1.45.1...v1.45.2) (2026-08-22)

### Bug Fixes

* **deps:** update dependency @tanstack/svelte-virtual to v3.13.36 ([#294](https://github.com/smykla-skalski/smyklot/issues/294)) ([27a9190](https://github.com/smykla-skalski/smyklot/commit/27a9190d80acbf8b8d63a3ec932be961b1626a24))
* **panel:** harden recent UI changes ([#298](https://github.com/smykla-skalski/smyklot/issues/298)) ([1e42fd5](https://github.com/smykla-skalski/smyklot/commit/1e42fd5e3f341ebcfbe2179c318a17f7a48b2731))

## [1.45.1](https://github.com/smykla-skalski/smyklot/compare/v1.45.0...v1.45.1) (2026-08-22)

### Bug Fixes

* **panel:** real avatars, fleet-proof sync strips, unclipped empty states ([#297](https://github.com/smykla-skalski/smyklot/issues/297)) ([563e6ef](https://github.com/smykla-skalski/smyklot/commit/563e6ef4ab1fccf2b2fe34cbb404d707b41edc7d))

## [1.45.0](https://github.com/smykla-skalski/smyklot/compare/v1.44.0...v1.45.0) (2026-08-21)

### Features

* **panel:** the shell redesign ([#292](https://github.com/smykla-skalski/smyklot/issues/292)) ([d9c1edb](https://github.com/smykla-skalski/smyklot/commit/d9c1edbc77313c4ab494e0e39a430fff8339d4b9)), closes [#e6ede9](https://github.com/smykla-skalski/smyklot/issues/e6ede9)

## [1.44.0](https://github.com/smykla-skalski/smyklot/compare/v1.43.1...v1.44.0) (2026-08-19)

### Features

* **sync:** the sync overhaul, and an index that rescans only what changed ([#282](https://github.com/smykla-skalski/smyklot/issues/282)) ([f3770d7](https://github.com/smykla-skalski/smyklot/commit/f3770d79f9835329950374c2c2d86b0dbefa4e36))

### Bug Fixes

* **deps:** update module modernc.org/sqlite to v1.57.0 ([#286](https://github.com/smykla-skalski/smyklot/issues/286)) ([ffc1ec1](https://github.com/smykla-skalski/smyklot/commit/ffc1ec118825e96f061fbf91a710cf3482b70b14))

## [1.43.1](https://github.com/smykla-skalski/smyklot/compare/v1.43.0...v1.43.1) (2026-08-18)

## [1.43.0](https://github.com/smykla-skalski/smyklot/compare/v1.42.0...v1.43.0) (2026-08-18)

## [1.42.0](https://github.com/smykla-skalski/smyklot/compare/v1.41.0...v1.42.0) (2026-08-18)

## [1.41.0](https://github.com/smykla-skalski/smyklot/compare/v1.40.0...v1.41.0) (2026-08-18)

## [1.40.0](https://github.com/smykla-skalski/smyklot/compare/v1.39.0...v1.40.0) (2026-08-18)

## [1.39.0](https://github.com/smykla-skalski/smyklot/compare/v1.38.0...v1.39.0) (2026-08-18)

## [1.38.0](https://github.com/smykla-skalski/smyklot/compare/v1.37.0...v1.38.0) (2026-08-18)

## [1.37.0](https://github.com/smykla-skalski/smyklot/compare/v1.36.0...v1.37.0) (2026-08-17)

## [1.36.0](https://github.com/smykla-skalski/smyklot/compare/v1.35.2...v1.36.0) (2026-08-17)

## [1.35.2](https://github.com/smykla-skalski/smyklot/compare/v1.35.1...v1.35.2) (2026-08-17)

## [1.35.1](https://github.com/smykla-skalski/smyklot/compare/v1.35.0...v1.35.1) (2026-08-17)

## [1.35.0](https://github.com/smykla-skalski/smyklot/compare/v1.34.0...v1.35.0) (2026-08-17)

## [1.34.0](https://github.com/smykla-skalski/smyklot/compare/v1.33.2...v1.34.0) (2026-08-17)

## [1.33.2](https://github.com/smykla-skalski/smyklot/compare/v1.33.1...v1.33.2) (2026-08-16)

## [1.33.1](https://github.com/smykla-skalski/smyklot/compare/v1.33.0...v1.33.1) (2026-08-16)

## [1.33.0](https://github.com/smykla-skalski/smyklot/compare/v1.32.0...v1.33.0) (2026-08-16)

## [1.32.0](https://github.com/smykla-skalski/smyklot/compare/v1.31.0...v1.32.0) (2026-08-16)

## [1.31.0](https://github.com/smykla-skalski/smyklot/compare/v1.30.2...v1.31.0) (2026-08-16)

## [1.30.2](https://github.com/smykla-skalski/smyklot/compare/v1.30.1...v1.30.2) (2026-08-16)

## [1.30.1](https://github.com/smykla-skalski/smyklot/compare/v1.30.0...v1.30.1) (2026-08-16)

## [1.30.0](https://github.com/smykla-skalski/smyklot/compare/v1.29.1...v1.30.0) (2026-08-16)

## [1.29.1](https://github.com/smykla-skalski/smyklot/compare/v1.29.0...v1.29.1) (2026-08-16)

## [1.29.0](https://github.com/smykla-skalski/smyklot/compare/v1.28.1...v1.29.0) (2026-08-16)

## [1.28.1](https://github.com/smykla-skalski/smyklot/compare/v1.28.0...v1.28.1) (2026-08-16)

## [1.28.0](https://github.com/smykla-skalski/smyklot/compare/v1.27.0...v1.28.0) (2026-08-16)

## [1.27.0](https://github.com/smykla-skalski/smyklot/compare/v1.26.0...v1.27.0) (2026-08-15)

## [1.26.0](https://github.com/smykla-skalski/smyklot/compare/v1.25.1...v1.26.0) (2026-08-15)

## [1.25.1](https://github.com/smykla-skalski/smyklot/compare/v1.25.0...v1.25.1) (2026-08-15)

## [1.25.0](https://github.com/smykla-skalski/smyklot/compare/v1.24.0...v1.25.0) (2026-08-15)

## [1.24.0](https://github.com/smykla-skalski/smyklot/compare/v1.23.2...v1.24.0) (2026-08-15)

## [1.23.2](https://github.com/smykla-skalski/smyklot/compare/v1.23.1...v1.23.2) (2026-08-15)

## [1.23.1](https://github.com/smykla-skalski/smyklot/compare/v1.23.0...v1.23.1) (2026-08-12)

## [1.23.0](https://github.com/smykla-skalski/smyklot/compare/v1.22.1...v1.23.0) (2026-08-12)

## [1.22.1](https://github.com/smykla-skalski/smyklot/compare/v1.22.0...v1.22.1) (2026-08-11)

## [1.22.0](https://github.com/smykla-skalski/smyklot/compare/v1.21.2...v1.22.0) (2026-08-11)

## [1.21.2](https://github.com/smykla-skalski/smyklot/compare/v1.21.1...v1.21.2) (2026-08-11)

## [1.21.1](https://github.com/smykla-skalski/smyklot/compare/v1.21.0...v1.21.1) (2026-08-11)

## [1.21.0](https://github.com/smykla-skalski/smyklot/compare/v1.20.0...v1.21.0) (2026-08-10)

## [1.20.0](https://github.com/smykla-skalski/smyklot/compare/v1.19.0...v1.20.0) (2026-08-10)

## [1.19.0](https://github.com/smykla-skalski/smyklot/compare/v1.18.0...v1.19.0) (2026-08-10)

## [1.18.0](https://github.com/smykla-skalski/smyklot/compare/v1.17.0...v1.18.0) (2026-08-10)

## [1.17.0](https://github.com/smykla-skalski/smyklot/compare/v1.16.0...v1.17.0) (2026-08-09)

## [1.16.0](https://github.com/smykla-skalski/smyklot/compare/v1.15.1...v1.16.0) (2026-08-09)

## [1.15.1](https://github.com/smykla-skalski/smyklot/compare/v1.15.0...v1.15.1) (2026-08-09)

## [1.15.0](https://github.com/smykla-skalski/smyklot/compare/v1.14.0...v1.15.0) (2026-08-09)

## [1.14.0](https://github.com/smykla-skalski/smyklot/compare/v1.13.0...v1.14.0) (2026-08-08)

## [1.13.0](https://github.com/smykla-skalski/smyklot/compare/v1.12.2...v1.13.0) (2026-08-08)

## [1.12.2](https://github.com/smykla-skalski/smyklot/compare/v1.12.1...v1.12.2) (2026-08-08)

## [1.12.1](https://github.com/smykla-skalski/smyklot/compare/v1.12.0...v1.12.1) (2026-06-21)

### Bug Fixes

* **ci:** bump create-github-app-token to v3.2.0 ([#99](https://github.com/smykla-skalski/smyklot/issues/99)) ([457d7dc](https://github.com/smykla-skalski/smyklot/commit/457d7dc03b13fb00a78e4fee69156499b2bd6794))
* **deps:** update ginkgo to v2.29.0 ([#92](https://github.com/smykla-skalski/smyklot/issues/92)) ([61837ea](https://github.com/smykla-skalski/smyklot/commit/61837eaf670db6a4265f0e614ed245036d573ea5))
* **deps:** update ginkgo to v2.30.0 ([#112](https://github.com/smykla-skalski/smyklot/issues/112)) ([198f6dd](https://github.com/smykla-skalski/smyklot/commit/198f6dd68b203bf254ba488762ace846cb2c21e6))
* **deps:** update module github.com/jferrl/go-githubauth to v1.6.0 ([#89](https://github.com/smykla-skalski/smyklot/issues/89)) ([30466f2](https://github.com/smykla-skalski/smyklot/commit/30466f2c9bf1a0bd60901156b97e447abfec6632))
* **deps:** update module github.com/onsi/gomega to v1.41.0 ([#93](https://github.com/smykla-skalski/smyklot/issues/93)) ([3cef9ab](https://github.com/smykla-skalski/smyklot/commit/3cef9ab38bf6c943ea32d3fc0103e6053e904f19))
* **deps:** update module github.com/onsi/gomega to v1.42.0 ([#113](https://github.com/smykla-skalski/smyklot/issues/113)) ([2635d40](https://github.com/smykla-skalski/smyklot/commit/2635d4094090bbc9e1bb3ec100c0ce66d097cf39))
* **lint:** extract repeated string literals to constants ([c81c508](https://github.com/smykla-skalski/smyklot/commit/c81c508760f3f24e78fd9fcb51aca67ad5bfc597))

## [1.12.0](https://github.com/smykla-skalski/smyklot/compare/v1.11.2...v1.12.0) (2026-04-13)

### Features

* **lint:** migrate to markdownlint-cli2 ([dbca17b](https://github.com/smykla-skalski/smyklot/commit/dbca17b7535f03178ee48b86029caf0f60006fd2))

### Bug Fixes

* **deps:** update ginkgo to v2.28.1 ([#57](https://github.com/smykla-skalski/smyklot/issues/57)) ([758ed1d](https://github.com/smykla-skalski/smyklot/commit/758ed1dfb912073a4c5410cbdce502e5b5ff5837))
* **deps:** update module github.com/jferrl/go-githubauth/v2 to v2.0.1 ([#82](https://github.com/smykla-skalski/smyklot/issues/82)) ([805a892](https://github.com/smykla-skalski/smyklot/commit/805a892f9c6d6a21b637194fad60cb96fb3d0981))
* **deps:** update module github.com/onsi/gomega to v1.39.0 ([#58](https://github.com/smykla-skalski/smyklot/issues/58)) ([6d3aa92](https://github.com/smykla-skalski/smyklot/commit/6d3aa928bd941b588361d2daa6e8500c2063cd9d))
* **deps:** update module github.com/onsi/gomega to v1.39.1 ([#68](https://github.com/smykla-skalski/smyklot/issues/68)) ([354caf4](https://github.com/smykla-skalski/smyklot/commit/354caf44e18b5c3c88ae55337ea18a8e45cc0f5b))
* **pkg:** use fmt.Fprintf, suppress G704 ([dfcb851](https://github.com/smykla-skalski/smyklot/commit/dfcb851e0b281b94fc7247fd1bdedeb7821d9e73))

### Code Refactoring

* **docs:** slim down CLAUDE.md ([29d09d4](https://github.com/smykla-skalski/smyklot/commit/29d09d46cb91bb638c060bb631764066bc7ad745))

## [1.11.2](https://github.com/smykla-skalski/smyklot/compare/v1.11.1...v1.11.2) (2026-02-10)

### Bug Fixes

* **action:** update registry path after rename ([7913acc](https://github.com/smykla-skalski/smyklot/commit/7913acce17b68262dd573842974904c8d246ae50))

## [1.11.0](https://github.com/smykla-skalski/smyklot/compare/v1.10.4...v1.11.0) (2025-12-08)

### Features

* **config:** add `quiet_pending` option ([#41](https://github.com/smykla-skalski/smyklot/issues/41)) ([eb902ba](https://github.com/smykla-skalski/smyklot/commit/eb902ba4e31e17e2b8ba48b50d7321b8cb66e09d))

## [1.10.4](https://github.com/smykla-skalski/smyklot/compare/v1.10.3...v1.10.4) (2025-12-08)

### Bug Fixes

* **sync-config:** disable smyklot workflow sync ([a73efe9](https://github.com/smykla-skalski/smyklot/commit/a73efe9ac99941d72c898b94770c52111ca0988b))

## [1.10.3](https://github.com/smykla-skalski/smyklot/compare/v1.10.2...v1.10.3) (2025-12-08)

### Bug Fixes

* **release:** fix client_payload JSON in gh api ([#39](https://github.com/smykla-skalski/smyklot/issues/39)) ([d8ba012](https://github.com/smykla-skalski/smyklot/commit/d8ba01254a4fbc3edf4c8a056a7a820da2e56ecb))

## [1.10.2](https://github.com/smykla-skalski/smyklot/compare/v1.10.1...v1.10.2) (2025-12-08)

### Bug Fixes

* **docker:** avoid QEMU emulation failures ([#38](https://github.com/smykla-skalski/smyklot/issues/38)) ([5430f15](https://github.com/smykla-skalski/smyklot/commit/5430f157c01085509ad2be0ff270a85d581d857b))

## [1.10.1](https://github.com/smykla-skalski/smyklot/compare/v1.10.0...v1.10.1) (2025-12-08)

### Bug Fixes

* **deps:** update module github.com/jferrl/go-githubauth to v2 ([#35](https://github.com/smykla-skalski/smyklot/issues/35)) ([8fcf3e6](https://github.com/smykla-skalski/smyklot/commit/8fcf3e69963459c215a1ba4feee8614e33e03f00))
* **deps:** update module github.com/spf13/cobra to v1.10.2 ([#34](https://github.com/smykla-skalski/smyklot/issues/34)) ([729b499](https://github.com/smykla-skalski/smyklot/commit/729b4998f7a0169161acb93d62813ee4620a6ef4))

## [1.10.0](https://github.com/smykla-skalski/smyklot/compare/v1.9.2...v1.10.0) (2025-12-06)

### Features

* **config:** add renovate.json sync configuration ([#32](https://github.com/smykla-skalski/smyklot/issues/32)) ([59382ea](https://github.com/smykla-skalski/smyklot/commit/59382ea04bff81e1355e2fbf92d0a4c5433fdfa5))

## [1.9.2](https://github.com/smykla-skalski/smyklot/compare/v1.9.1...v1.9.2) (2025-12-04)

### Bug Fixes

* **github:** use valid reaction for pending CI ([#29](https://github.com/smykla-skalski/smyklot/issues/29)) ([7659f4f](https://github.com/smykla-skalski/smyklot/commit/7659f4f30211bfb6281ca4da10cef0569e210b5e))

## [1.9.1](https://github.com/smykla-skalski/smyklot/compare/v1.9.0...v1.9.1) (2025-12-04)

### Code Refactoring

* **repo:** migrate to smykla-skalski org ([#27](https://github.com/smykla-skalski/smyklot/issues/27)) ([1e22f09](https://github.com/smykla-skalski/smyklot/commit/1e22f09a0ba524019d8cbe64ea126f9a21e5c804))

## [1.9.0](https://github.com/bartsmykla/smyklot/compare/v1.8.0...v1.9.0) (2025-12-04)

### Features

* **commands:** add CI terms and required checks ([#26](https://github.com/bartsmykla/smyklot/issues/26)) ([81b55f0](https://github.com/bartsmykla/smyklot/commit/81b55f0a8f4a88891b4cb4b1ada2b24bec10e1bd))

## [1.8.0](https://github.com/bartsmykla/smyklot/compare/v1.7.14...v1.8.0) (2025-12-03)

### Features

* **commands:** add merge after CI modifier ([#11](https://github.com/bartsmykla/smyklot/issues/11)) ([31cef02](https://github.com/bartsmykla/smyklot/commit/31cef02f3da1b31a090c3cd42710c5b68f0c94f9))
