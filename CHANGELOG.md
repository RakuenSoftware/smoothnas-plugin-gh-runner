# Changelog

## [0.3.38](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.37...v0.3.38) (2026-05-22)


### Bug Fixes

* bake Atomic llama.cpp source into runner image

## [0.3.37](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.36...v0.3.37) (2026-05-22)


### Bug Fixes

* bake accelerator runner tools ([#88](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/88)) ([18718e1](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/18718e1506644b9d18b03985dc4af17f86603681))

## [0.3.36](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.35...v0.3.36) (2026-05-22)


### Bug Fixes

* point release manifest at current runner image tag

## [0.3.35](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.34...v0.3.35) (2026-05-22)


### Bug Fixes

* restore action node runtimes from chunks ([#85](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/85)) ([62dcafa](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/62dcafa))

## [0.3.34](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.33...v0.3.34) (2026-05-21)


### Bug Fixes

* bake action node runtime backups ([#82](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/82)) ([1341758](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/1341758))

## [0.3.33](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.32...v0.3.33) (2026-05-21)


### Bug Fixes

* restore action node runtimes from job hook ([#80](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/80)) ([3cf13f8](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/3cf13f81a2fa488e344e2d52356ff9457a6da3fa))

## [0.3.32](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.31...v0.3.32) (2026-05-21)


### Bug Fixes

* create runner workspace from job start hook ([#78](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/78)) ([5e59897](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/5e59897afc815fa1d9b5762aa948173badfd2aa6))

## [0.3.31](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.30...v0.3.31) (2026-05-21)


### Bug Fixes

* precreate workspaces before runner registration ([#76](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/76)) ([5f30439](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/5f304393c57be01de105558edd76b5161e2210bc))

## [0.3.30](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.29...v0.3.30) (2026-05-21)


### Bug Fixes

* prepare self-hosted runner workspaces ([#74](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/74)) ([9547494](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/9547494c027d59ae5d3253b2496796a04ba24a33))

## [0.3.29](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.28...v0.3.29) (2026-05-21)


### Bug Fixes

* allow runner without ICU data ([#72](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/72)) ([ef87f3a](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/ef87f3a65ff75c6f0014821756b6a654547b021b))

## [0.3.28](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.27...v0.3.28) (2026-05-21)


### Bug Fixes

* download action node runtime when backups are missing ([20c3e2e](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/20c3e2e4f64f1c2f52e77acb1e04bbae76301dbf))
* download action node runtime when backups are missing ([8669415](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/8669415eb9a9338b1d1ddf667798253d8b126b97))

## [0.3.27](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.26...v0.3.27) (2026-05-21)


### Bug Fixes

* repair action node runtimes at worker start ([b52f92d](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/b52f92d38cc3887335f3b768cd0b82d64368b8cf))

## [0.3.26](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.25...v0.3.26) (2026-05-21)


### Bug Fixes

* keep action node runtimes in runner tree ([dd342ca](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/dd342cabc88c99295188f0330a542f43fb77c14f))

## [0.3.25](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.24...v0.3.25) (2026-05-21)


### Bug Fixes

* detect controller Docker socket ([#64](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/64)) ([769e79c](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/769e79cefe244308cfe7b5f0ca452340e12a1fea))

## [0.3.24](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.23...v0.3.24) (2026-05-21)


### Bug Fixes

* use SmoothNAS runtime socket for workers ([#62](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/62)) ([0dfb0d6](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/0dfb0d650883c1ecbc10e3ac23be7e66c453e728))

## [0.3.23](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.22...v0.3.23) (2026-05-21)


### Bug Fixes

* equip self-hosted workers for builds ([#60](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/60)) ([c1de6b9](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/c1de6b96e8136be460462d1d1ab24d28249b6502))

## [0.3.22](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.21...v0.3.22) (2026-05-21)


### Bug Fixes

* remove runner node runtime restore ([#58](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/58)) ([534583a](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/534583ab40d296b048f65515b5451bca2bc61836))

## [0.3.21](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.20...v0.3.21) (2026-05-20)


### Bug Fixes

* continuously repair action node runtimes ([#56](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/56)) ([93d487d](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/93d487d7fbd869b9306826a4740464257423e908))

## [0.3.20](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.19...v0.3.20) (2026-05-20)


### Bug Fixes

* restore action node runtimes in workers ([#54](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/54)) ([6db0c56](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/6db0c56d55a5953c1b56c8bc637cef8717fc80cd))

## [0.3.19](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.18...v0.3.19) (2026-05-20)


### Bug Fixes

* remove unsupported CPU resource from manifest ([#52](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/52)) ([a702f7a](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/a702f7afbb21e1d5befe06596c119a627daf11d6))

## [0.3.18](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.17...v0.3.18) (2026-05-20)


### Bug Fixes

* release gh-runner node whiteout fix ([#50](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/50)) ([10e9528](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/10e95284e25706d2ac3d81e8fcf86f51bc755501))

## [0.3.17](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.16...v0.3.17) (2026-05-20)


### Bug Fixes

* keep runner node binaries in base layer ([#47](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/47)) ([f687efa](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/f687efa3cabb4bb1adc329208809812d38bcc464))
* rotate workers after image upgrades ([#45](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/45)) ([79dc73e](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/79dc73e8740ae2597ba00c89c49961d73b1b4025))

## [0.3.16](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.15...v0.3.16) (2026-05-20)


### Bug Fixes

* rotate workers with stale resource limits ([#42](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/42)) ([228cfa5](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/228cfa5a1648f9e0fa446b9cbcee30e90c97c52d))
* run gh-runner workers on debian 13 ([#44](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/44)) ([58f59bf](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/58f59bf153c837d248f6232dc566ffec102c9e2a))

## [0.3.15](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.14...v0.3.15) (2026-05-20)


### Bug Fixes

* shrink idle runner workers ([1130d45](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/1130d45207ae82358d712ba2a65c34dcd3135bdd))
* shrink idle runner workers ([1a3e0bc](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/1a3e0bcdcf08b347c551b86123d0940cac4b9b60))

## [0.3.14](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.13...v0.3.14) (2026-05-20)


### Bug Fixes

* bundle executable action node runtimes ([65d04e9](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/65d04e9be9a2a747f0af23282d804e2f487ac7b0))
* include xz for node runtime install ([c4c445d](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/c4c445df3fe14f9fb48022661f65fb7a03e9c6ae))

## [0.3.13](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.12...v0.3.13) (2026-05-20)


### Bug Fixes

* require releasable commits before merge ([f629f61](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/f629f61a6c921fc046ab6cc05b0a4aa79a85417d))
* require releasable commits before merge ([ce15345](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/ce1534572cce8d5474c3f515a161a675f4085b3f))

## [0.3.12](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.11...v0.3.12) (2026-05-18)


### Bug Fixes

* recycle orphaned runner workers ([#31](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/31)) ([929d59f](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/929d59fbc09ee87982dc7e6ffc411a29b250b47e))
* remove offline runner registrations ([#33](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/33)) ([7f486d7](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/7f486d7f3dfe76f5f6069cd941e31455c5a93826))

## [0.3.11](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.10...v0.3.11) (2026-05-18)


### Bug Fixes

* preserve runtime dns by default ([#29](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/29)) ([bdd4a0d](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/bdd4a0defd113aafa57a57d044d07c5fc2860379))

## [0.3.10](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.9...v0.3.10) (2026-05-17)


### Bug Fixes

* default to one runner worker ([f3b4cf5](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/f3b4cf515d2452249b87c38dabf1441f81895f46))
* default to one runner worker ([8ad6334](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/8ad6334daffc859a12737a51fc40e8d9aa9a9caf))

## [0.3.9](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.8...v0.3.9) (2026-05-17)


### Bug Fixes

* stabilize runner container dns ([b17589e](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/b17589e1851218a496160d314cc00957abf223ab))
* stabilize runner container dns ([ebaf2c8](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/ebaf2c8129833fd83523f7f59984e6b7d0247739))

## [0.3.8](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.7...v0.3.8) (2026-05-17)


### Bug Fixes

* avoid reaping unregistered workers during cleanup ([59bd33f](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/59bd33f5d7ad2fce21be83dcd68fda11f77ffaa6))
* avoid reaping unregistered workers during cleanup ([fdcaac2](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/fdcaac2c3a2683e6618b41a623326359b200027d))

## [0.3.7](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.6...v0.3.7) (2026-05-17)


### Bug Fixes

* preserve busy runners during cleanup ([ead3336](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/ead3336df1412ae471ab6db7fa5d4b02fdcc0915))
* preserve busy runners during cleanup ([999c949](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/999c949316fcd3981aa6ced166360fde1240a3db))

## [0.3.6](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.5...v0.3.6) (2026-05-17)


### Bug Fixes

* reap offline runner workers ([#19](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/19)) ([6b96cc3](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/6b96cc39f39f0463fdc906389ab5d395c6db9fee))

## [0.3.5](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.4...v0.3.5) (2026-05-17)


### Bug Fixes

* dispatch release workflow with repo ([#17](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/17)) ([838d202](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/838d20274d1a4066dfa4a73d4e95329b1edbfcd7))

## [0.3.4](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.3...v0.3.4) (2026-05-17)


### Bug Fixes

* dispatch plugin release publisher ([#15](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/15)) ([3fc6ae3](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/3fc6ae3bd4325fccaedc5e22bd930a84a86b3c73))

## [0.3.3](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.2...v0.3.3) (2026-05-17)


### Bug Fixes

* clean stale github runner registrations ([#13](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/issues/13)) ([87d9b86](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/87d9b865f23b9a5a50d92106719f8ec26e83f5b7))

## [0.3.2](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.1...v0.3.2) (2026-05-17)


### Bug Fixes

* use ephemeral worker workspaces by default ([408195e](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/408195e5ff4adae9b28b7113dbf61912d46a7c62))

## [0.3.1](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.3.0...v0.3.1) (2026-05-17)


### Bug Fixes

* accept GitHub OAuth runner tokens ([2add1a5](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/2add1a50c95a96a239753ab8b767045506efcca1))

## [0.3.0](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.2.3...v0.3.0) (2026-05-17)


### Features

* run github actions workers ephemerally ([91dd302](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/91dd302dac42cda9a36cc5e1f86490e50b2d7234))

## [0.2.3](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.2.2...v0.2.3) (2026-05-17)


### Bug Fixes

* stabilize runner startup ([334b9c0](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/334b9c0213d7cd4f880544ce9fb5713b7757a82b))

## [0.2.2](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.2.1...v0.2.2) (2026-05-16)


### Bug Fixes

* allow GitHub runner setup under SmoothNAS LXC runtime

## [0.2.1](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.2.0...v0.2.1) (2026-05-16)


### Bug Fixes

* publish image with current actions runner asset names

## [0.2.0](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/compare/v0.1.0...v0.2.0) (2026-05-08)


### Features

* auto-version on merges to main via release-please ([561cc8c](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/561cc8c03bfa5c125abc58482d0aaf050f0c6f35))
* auto-version on merges to main via release-please ([e7820c3](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/e7820c36ddc7824e1ba37f064270d550b62e0fcd))
* auto-version on merges to main via release-please (re-PR) ([2b46683](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/2b46683ac80700bd7dfed57311e711d6713ff4a0))


### Bug Fixes

* lowercase the GHCR tag so buildx push doesn't 400 ([351c10e](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/351c10ea0274cf66e0a33b596a07c497a89d8afe))
* lowercase the GHCR tag so buildx push doesn't 400 ([eb3d109](https://github.com/RakuenSoftware/smoothnas-plugin-gh-runner/commit/eb3d109850f2f54fa5d6db1bda611f7b9713aa25))
