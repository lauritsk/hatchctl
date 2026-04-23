# Changelog
All notable changes to this project will be documented in this file. See [conventional commits](https://www.conventionalcommits.org/) for commit guidelines.

- - -
## [v0.8.4](https://github.com/lauritsk/hatchctl/compare/19eb55d304a405f399babb14a199296fe80ae8ba..v0.8.4) - 2026-04-23
#### Bug Fixes
- ensure mise gets installed for all actions - ([19eb55d](https://github.com/lauritsk/hatchctl/commit/19eb55d304a405f399babb14a199296fe80ae8ba)) - Karl Hans Laurits

- - -

## [v0.8.3](https://github.com/lauritsk/hatchctl/compare/b4f9e5de237e93d7f9c325460a7595b0fa0657b4..v0.8.3) - 2026-04-23
#### Bug Fixes
- (**formatting**) rename ci to check and space out files - ([259636a](https://github.com/lauritsk/hatchctl/commit/259636a19934d2d641a1e3318aee9f12a8a0cd62)) - Karl Hans Laurits
- remove quotes from github actions on element - ([6efc262](https://github.com/lauritsk/hatchctl/commit/6efc26202d6f13c6a34b9bdbc8f7b9818716c162)) - Karl Hans Laurits
#### Documentation
- add contributing guide - ([2108333](https://github.com/lauritsk/hatchctl/commit/210833329525479dc5496a927abe75786af774b3)) - Karl Hans Laurits
- rewrite README - ([cd71b8c](https://github.com/lauritsk/hatchctl/commit/cd71b8cedd97e674aeabafbdac0b6c1f10473ec6)) - Karl Hans Laurits
#### Refactoring
- inline release helper build and simplify hk tasks - ([c9dc8fd](https://github.com/lauritsk/hatchctl/commit/c9dc8fd68e6248f0b2fb5d40de3f8c7ce1b27c69)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**tooling**) simplify release orchestration - ([1dda178](https://github.com/lauritsk/hatchctl/commit/1dda1780f47a0d05bb7971a1f3dce598dd0d24c8)) - Karl Hans Laurits
- (**tooling**) move goreleaser checks into hk and remove docs - ([02ce686](https://github.com/lauritsk/hatchctl/commit/02ce6862e74da85b729454432ce328dbe2581a25)) - Karl Hans Laurits
- (**tooling**) remove manual sbom task - ([040b981](https://github.com/lauritsk/hatchctl/commit/040b981e9da45ab87dd22c17369bd86106dac166)) - Karl Hans Laurits
- (**tooling**) simplify mise tasks around hk - ([ffca9ab](https://github.com/lauritsk/hatchctl/commit/ffca9ab1548219896d937c6859edad6e0c622d64)) - Karl Hans Laurits
- (**tooling**) add hk-managed trivy scan - ([da9f599](https://github.com/lauritsk/hatchctl/commit/da9f5993c37a2c53a533e504a2bedafcfbc08983)) - Karl Hans Laurits
- (**tooling**) reorder hk checks by scope - ([a0dafb1](https://github.com/lauritsk/hatchctl/commit/a0dafb1207b1391f71375c13070a5009e15f132d)) - Karl Hans Laurits
- (**tooling**) expand hk builtins and update contributing - ([2633655](https://github.com/lauritsk/hatchctl/commit/26336556163ef0fd2c19f978fc2532615957f697)) - Karl Hans Laurits
- remove vendored go modules - ([1ac7cc2](https://github.com/lauritsk/hatchctl/commit/1ac7cc2cf5f365f47672740f59d16af011010a52)) - Karl Hans Laurits
- simplify setup and slow hk checks - ([519932f](https://github.com/lauritsk/hatchctl/commit/519932fde66f949e628b3bdddd88d555ff782a94)) - Karl Hans Laurits
- simplify format task with hk - ([f65d485](https://github.com/lauritsk/hatchctl/commit/f65d48569d3c05fa4ade6bdb5b7396d0314872d4)) - Karl Hans Laurits
- simplify lint tasks with hk - ([53b870b](https://github.com/lauritsk/hatchctl/commit/53b870bad1a87cf9bcbc9a5e75f89224e7db648b)) - Karl Hans Laurits
- simplify hk hook setup - ([91ed923](https://github.com/lauritsk/hatchctl/commit/91ed9236b87d3e8e1550ca9c52008c3fb21e7813)) - Karl Hans Laurits
- simplify mise tasks - ([e61fde0](https://github.com/lauritsk/hatchctl/commit/e61fde01c24258fef05672c06178b7d7cb174126)) - Karl Hans Laurits
- add devcontainer config - ([64ae1a8](https://github.com/lauritsk/hatchctl/commit/64ae1a8605ff47699b9f6c5f71c930cd1af7599b)) - Karl Hans Laurits
- standardize repo automation - ([b4f9e5d](https://github.com/lauritsk/hatchctl/commit/b4f9e5de237e93d7f9c325460a7595b0fa0657b4)) - Karl Hans Laurits

- - -

## [v0.8.2](https://github.com/lauritsk/hatchctl/compare/e040fd1bae25797eb5a9d6000eb309991565c604..v0.8.2) - 2026-04-22
#### Bug Fixes
- (**deps**) update module charm.land/lipgloss/v2 to v2.0.3 - ([55db8a5](https://github.com/lauritsk/hatchctl/commit/55db8a5abc2b870630a79f1bb2851539fa4208ca)) - renovate[bot]
- (**runtime**) Return fallback compose config errors - ([a084d75](https://github.com/lauritsk/hatchctl/commit/a084d75c45f9e8ed199f92e53be4cbab5328c0a2)) - Karl Hans Laurits
- (**runtime**) Surface workspace lock release failures - ([1ecb1cb](https://github.com/lauritsk/hatchctl/commit/1ecb1cbb3a936a1f5df877b255a136c823729391)) - Karl Hans Laurits
- (**runtime**) Revalidate managed container reads - ([e040fd1](https://github.com/lauritsk/hatchctl/commit/e040fd1bae25797eb5a9d6000eb309991565c604)) - Karl Hans Laurits
#### Documentation
- remove temporary backlog file - ([591653f](https://github.com/lauritsk/hatchctl/commit/591653f596c6d221c771d041c387866e86e8bdda)) - Karl Hans Laurits
#### Refactoring
- (**devcontainer**) Move spec implementation into devcontainer - ([edb185e](https://github.com/lauritsk/hatchctl/commit/edb185ee87770be3a1914c794daf77caf4da6353)) - Karl Hans Laurits
- (**devcontainer**) Move spec implementation into devcontainer - ([695ed58](https://github.com/lauritsk/hatchctl/commit/695ed58bba12364338c331d246d2a098a10e9bc3)) - Karl Hans Laurits
- (**runtime**) Share bridge process liveness checks - ([a4aa302](https://github.com/lauritsk/hatchctl/commit/a4aa302a5ad3047f59be412a78e1276ae0ece3df)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**deps**) update go lockfile - ([bb8c084](https://github.com/lauritsk/hatchctl/commit/bb8c0842106b88198a474e34e76b0597455fefe5)) - Karl Hans Laurits
- (**deps**) update alpine docker tag to v3.23.4 - ([1321c0b](https://github.com/lauritsk/hatchctl/commit/1321c0b9df1692bbaa67de480cd10452457523d0)) - renovate[bot]
- (**deps**) update module github.com/sigstore/timestamp-authority/v2 to v2.0.6 [security] - ([5d840c2](https://github.com/lauritsk/hatchctl/commit/5d840c2e736f12a1460db5fc276e177dbe032444)) - renovate[bot]
- (**deps**) update module go:golang.org/x/vuln/cmd/govulncheck to v1.2.0 - ([250ab8a](https://github.com/lauritsk/hatchctl/commit/250ab8afc94298c6dd1f389f59f97fb02a853029)) - renovate[bot]
- (**deps**) update dependency github:lauritsk/hatchctl to v0.8.1 - ([e6b026d](https://github.com/lauritsk/hatchctl/commit/e6b026d726f80e46bf9f36ab599ca51daa0de1c4)) - renovate[bot]
- (**deps**) update actions/cache action to v5.0.5 - ([1a780d9](https://github.com/lauritsk/hatchctl/commit/1a780d9fb7db7e9d2cb2d10d7508fb10049bbd31)) - renovate[bot]
- initialise beads for this project - ([efce37a](https://github.com/lauritsk/hatchctl/commit/efce37a8908f8d978475ed5fdf60fe34c9e1fa47)) - Karl Hans Laurits
- update dependencies and vendor go modules into project - ([3a536e6](https://github.com/lauritsk/hatchctl/commit/3a536e6034fb21322ad9a525c8263f709d41d459)) - Karl Hans Laurits

- - -

## [v0.8.1](https://github.com/lauritsk/hatchctl/compare/5bbe596ab4801d0b7ed5e4637834b6f66517a810..v0.8.1) - 2026-04-14
#### Bug Fixes
- (**runtime**) Restore source image metadata fallback - ([fb9233d](https://github.com/lauritsk/hatchctl/commit/fb9233d3a9934a2c6a8deee1d3ae6fba29d32d9a)) - Karl Hans Laurits
#### Refactoring
- (**cli**) Share command setup helpers - ([ec761ec](https://github.com/lauritsk/hatchctl/commit/ec761ec07520a52e465f7e23c564d5d64d7a85ec)) - Karl Hans Laurits
- (**runtime**) Keep helper ownership package-local - ([e5474e7](https://github.com/lauritsk/hatchctl/commit/e5474e77e04415de8fa7a97cd408bf2bc2d2b15a)) - Karl Hans Laurits
- (**runtime**) Collapse state tracker persist helpers - ([7eb5877](https://github.com/lauritsk/hatchctl/commit/7eb587732795647886a7dede36c7f1e52c2e0096)) - Karl Hans Laurits
- (**runtime**) Share bridge and compose helpers - ([e63bba2](https://github.com/lauritsk/hatchctl/commit/e63bba2441cf4d0547f005832214869598cb9e7c)) - Karl Hans Laurits
- (**runtime**) Route state writes through tracker - ([96cac1d](https://github.com/lauritsk/hatchctl/commit/96cac1d802b326b75470360d70f700c46e5463c0)) - Karl Hans Laurits
- (**runtime**) Centralize runtime metadata enrichment - ([5bbe596](https://github.com/lauritsk/hatchctl/commit/5bbe596ab4801d0b7ed5e4637834b6f66517a810)) - Karl Hans Laurits
- (**store**) Centralize store file writers - ([bb828c7](https://github.com/lauritsk/hatchctl/commit/bb828c72043c5deeebe6135c638c14cf4637eda1)) - Karl Hans Laurits

- - -

## [v0.8.0](https://github.com/lauritsk/hatchctl/compare/5b66255e77a33fbfebd2e9545f811d4c52744ed7..v0.8.0) - 2026-04-13
#### Features
- (**runtime**) Add bridge host fallbacks - ([af0f8dc](https://github.com/lauritsk/hatchctl/commit/af0f8dcb3e3fdf7bec02ad291d04ce4399cc2677)) - Karl Hans Laurits
- (**runtime**) Add backend capability gating - ([809c020](https://github.com/lauritsk/hatchctl/commit/809c020c8fd620776eead35ae7c51fedaad028e2)) - Karl Hans Laurits
- (**runtime**) Support podman compose variants - ([8b83ed9](https://github.com/lauritsk/hatchctl/commit/8b83ed9955fec98e87a7b56f3a2b693fd6a44a12)) - Karl Hans Laurits
- (**runtime**) Add auto backend detection - ([5b66255](https://github.com/lauritsk/hatchctl/commit/5b66255e77a33fbfebd2e9545f811d4c52744ed7)) - Karl Hans Laurits
#### Bug Fixes
- (**runtime**) Handle podman image not-found errors - ([52eae51](https://github.com/lauritsk/hatchctl/commit/52eae511e0e5488a964bacccd67ae4c91068a83f)) - Karl Hans Laurits
#### Tests
- (**runtime**) Add podman integration coverage - ([ab25c19](https://github.com/lauritsk/hatchctl/commit/ab25c19c37f4943d3cce7170228e8f95573dcf7c)) - Karl Hans Laurits

- - -

## [v0.7.0](https://github.com/lauritsk/hatchctl/compare/af8aa53ee58bd14a826412d6f34e95502d7da016..v0.7.0) - 2026-04-13
#### Features
- (**runtime**) Support Containerfile defaults - ([033b0bc](https://github.com/lauritsk/hatchctl/commit/033b0bcfdd1359b23bcca1b57a5d1d439c6d0576)) - Karl Hans Laurits
- (**runtime**) Add podman backend support - ([f27c1e0](https://github.com/lauritsk/hatchctl/commit/f27c1e07dcf25c1419cef0b4e66ea183efc219e5)) - Karl Hans Laurits
#### Refactoring
- (**runtime**) Introduce backend-neutral runtime - ([b4b3b00](https://github.com/lauritsk/hatchctl/commit/b4b3b0085988817ce2c46d1c2b32c11066fedda9)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([af8aa53](https://github.com/lauritsk/hatchctl/commit/af8aa53ee58bd14a826412d6f34e95502d7da016)) - Karl Hans Laurits

- - -

## [v0.6.17](https://github.com/lauritsk/hatchctl/compare/e0c51207336287928c65b9aaea91c33f4eba3cae..v0.6.17) - 2026-04-12
#### Bug Fixes
- (**devcontainer**) Mount mise volumes in dev home - ([3143c75](https://github.com/lauritsk/hatchctl/commit/3143c75897ae5d87e46b1c96c7537ea03cae0fc7)) - Karl Hans Laurits
- (**runtime**) Reduce failures around mutable workspace state - ([e0c5120](https://github.com/lauritsk/hatchctl/commit/e0c51207336287928c65b9aaea91c33f4eba3cae)) - Karl Hans Laurits

- - -

## [v0.6.16](https://github.com/lauritsk/hatchctl/compare/1c5c8be56e971febada613036ba11a44eaf0d190..v0.6.16) - 2026-04-12
#### Bug Fixes
- Harden workspace locking and feature resolution - ([3469db4](https://github.com/lauritsk/hatchctl/commit/3469db41daeb402d96d295092b69bcc019b6a52d)) - Karl Hans Laurits
#### Documentation
- Refresh README and agent guidance - ([6a62c3d](https://github.com/lauritsk/hatchctl/commit/6a62c3dee97bf1d75e4edd7079e11419e301b017)) - Karl Hans Laurits
- Clarify CLI docs and trust defaults - ([8e07a16](https://github.com/lauritsk/hatchctl/commit/8e07a165b639238d7fd85a98e4932e22530c1dff)) - Karl Hans Laurits
#### Refactoring
- (**featurefetch**) Split OCI registry fetch helpers - ([386bb8e](https://github.com/lauritsk/hatchctl/commit/386bb8e20204f0cef0494f374f68e3a6e5aebdfd)) - Karl Hans Laurits
- (**featurefetch**) Split feature source parsing helpers - ([094dfcb](https://github.com/lauritsk/hatchctl/commit/094dfcb9925852e78f3a0619723feaa80da566b6)) - Karl Hans Laurits
- (**reconcile**) Reduce reconcile test fixture duplication - ([be2ad53](https://github.com/lauritsk/hatchctl/commit/be2ad5311f1651ec21cc8007b6903bfc78f930d5)) - Karl Hans Laurits
- (**reconcile**) Avoid duplicate bridge inspection in config - ([29f497d](https://github.com/lauritsk/hatchctl/commit/29f497d69b8d8663dbba987d947449bbb72032e2)) - Karl Hans Laurits
- Simplify CLI wiring and workspace prep - ([fc88843](https://github.com/lauritsk/hatchctl/commit/fc88843ec6cb7558209a7a1a43295043a9d79c64)) - Karl Hans Laurits
- Simplify internal helpers - ([1c5c8be](https://github.com/lauritsk/hatchctl/commit/1c5c8be56e971febada613036ba11a44eaf0d190)) - Karl Hans Laurits

- - -

## [v0.6.15](https://github.com/lauritsk/hatchctl/compare/ed52a3587b118bd2a2026c54a11fb2c94ef74f48..v0.6.15) - 2026-04-12
#### Bug Fixes
- Avoid flaky docker client test runner - ([b908d09](https://github.com/lauritsk/hatchctl/commit/b908d097ab492966e2ad20491f499b7fc8900284)) - Karl Hans Laurits
#### Tests
- Harden bridge listener shutdown check - ([8773a96](https://github.com/lauritsk/hatchctl/commit/8773a96ef7b6ad06768aeca0fb362dbb10958eff)) - Karl Hans Laurits
- Stabilize bridge runtime port test - ([f21630c](https://github.com/lauritsk/hatchctl/commit/f21630cbd48411ea3af968e65b355a663299fa47)) - Karl Hans Laurits
#### Refactoring
- Drop config type aliases - ([a8f6ddc](https://github.com/lauritsk/hatchctl/commit/a8f6ddc889c24ff098de8973d7962c7f2b064cc9)) - Karl Hans Laurits
- Drop merged metadata aliases - ([e5505fc](https://github.com/lauritsk/hatchctl/commit/e5505fc007f5730d02e0ec0db38d783b5a412f36)) - Karl Hans Laurits
- Drop lifecycle command aliases - ([8fd0e62](https://github.com/lauritsk/hatchctl/commit/8fd0e626ebbf5313a2595ad3810f0926d2ea679e)) - Karl Hans Laurits
- Drop devcontainer state aliases - ([d14a168](https://github.com/lauritsk/hatchctl/commit/d14a16836db7172286596dd4aa75fc2d9bafeecd)) - Karl Hans Laurits
- Trim devcontainer facade helpers - ([c61f7c3](https://github.com/lauritsk/hatchctl/commit/c61f7c34c6508fd1676be9b30dddd8854a23e25e)) - Karl Hans Laurits
- Reuse planned workspace inputs - ([4f7f043](https://github.com/lauritsk/hatchctl/commit/4f7f0437fb0b6c1bc9eb729e008cb47be2189d4b)) - Karl Hans Laurits
- Remove thin devcontainer spec wrappers - ([563effc](https://github.com/lauritsk/hatchctl/commit/563effc4bc0f66f8ff8baec236e5aa698838a2af)) - Karl Hans Laurits
- Reduce duplicated app and CLI setup - ([ed52a35](https://github.com/lauritsk/hatchctl/commit/ed52a3587b118bd2a2026c54a11fb2c94ef74f48)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**devcontainer**) Persist mise tool caches across recreates - ([0ec410a](https://github.com/lauritsk/hatchctl/commit/0ec410a413d14e445511933fe27d917c7a15fa76)) - Karl Hans Laurits

- - -

## [v0.6.14](https://github.com/lauritsk/hatchctl/compare/4ed5d633958225e38951c444ef193e20fbd1d428..v0.6.14) - 2026-04-11
#### Bug Fixes
- (**deps**) update module github.com/google/go-containerregistry to v0.21.5 - ([9ac0c77](https://github.com/lauritsk/hatchctl/commit/9ac0c7782730a64cbd2abf113f97d4d9ea9d578e)) - renovate[bot]
- Gate repo-local host path defaults behind workspace trust - ([da32c15](https://github.com/lauritsk/hatchctl/commit/da32c155b628e77190465be8510e5767bf00e37a)) - Karl Hans Laurits
#### Tests
- (**runtime**) Reduce regression risk in bridge and reconcile workflows - ([9b92285](https://github.com/lauritsk/hatchctl/commit/9b9228539b3a1eb6d1d1e282abad088f7c52b68e)) - Karl Hans Laurits
- Increase coverage for reconcile helpers - ([b06742b](https://github.com/lauritsk/hatchctl/commit/b06742bdc484cd8a5f4e113963f7e0f11d1f15b8)) - Karl Hans Laurits
- Increase coverage for renderer and spec helpers - ([a71ff65](https://github.com/lauritsk/hatchctl/commit/a71ff65d5222eab1863c8451b427a2b350438b84)) - Karl Hans Laurits
- Increase coverage for trust and storage helpers - ([e888e5e](https://github.com/lauritsk/hatchctl/commit/e888e5e046472bc39c8dc8c0e25f54b04026bb9a)) - Karl Hans Laurits
#### Refactoring
- Simplify remote feature source resolution branches - ([62b8c2c](https://github.com/lauritsk/hatchctl/commit/62b8c2c9b689d863d747fd9e137f9712e5ba595c)) - Karl Hans Laurits
- Make the devcontainer resolve flow easier to follow - ([d027f2c](https://github.com/lauritsk/hatchctl/commit/d027f2c3b6c3fa9ccb9ecc682705fe4982de1da3)) - Karl Hans Laurits
- Split config and runtime workflows into focused helpers - ([a60d17a](https://github.com/lauritsk/hatchctl/commit/a60d17ae1c44f13e22ad31d0f4bba6fd446c3f6e)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**config**) migrate config renovate.json - ([1d31adc](https://github.com/lauritsk/hatchctl/commit/1d31adc655c6c670e5359f6a236a947f1814c9d0)) - renovate[bot]
- (**deps**) update actions/upload-artifact action to v7 - ([f8c956f](https://github.com/lauritsk/hatchctl/commit/f8c956f989689d7c20d83fcc65d08098e1d54392)) - renovate[bot]
- (**deps**) Remove stale module checksums - ([4eada1b](https://github.com/lauritsk/hatchctl/commit/4eada1bae8a0d00210ab3a0c28d4a1ed42b88378)) - Karl Hans Laurits
- (**deps**) update actions/upload-artifact action to v6 - ([1ab2e8f](https://github.com/lauritsk/hatchctl/commit/1ab2e8f33e5bd2588fc659d5ddaef44e21770edc)) - renovate[bot]
- (**devcontainer**) update devcontainer image to v0.2.0 - ([14b44bd](https://github.com/lauritsk/hatchctl/commit/14b44bd826adc236bc7d4e9b19cec67f73ea44b4)) - Karl Hans Laurits
- (**mise**) Update hatchctl to v0.6.13 - ([4ed5d63](https://github.com/lauritsk/hatchctl/commit/4ed5d633958225e38951c444ef193e20fbd1d428)) - Karl Hans Laurits

- - -

## [v0.6.13](https://github.com/lauritsk/hatchctl/compare/9621cb901dec10c92b114a20dd9cac630644d393..v0.6.13) - 2026-04-10
#### Bug Fixes
- (**ci**) Batch workflow output writes for shellcheck - ([eb400d6](https://github.com/lauritsk/hatchctl/commit/eb400d6e25a462cb811402287bdaf88d939204bf)) - Karl Hans Laurits
- (**security**) Verify pinned images with configurable trusted signers - ([14235b0](https://github.com/lauritsk/hatchctl/commit/14235b0090b4423878d20462b92c08a6ce2f06b5)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**ci**) Pin workflow runners and container refs - ([06286c0](https://github.com/lauritsk/hatchctl/commit/06286c0095ccf8e3cee4318c7cd7ef2c6a2aee2f)) - Karl Hans Laurits
- (**mise**) Add shellcheck for workflow linting - ([6984058](https://github.com/lauritsk/hatchctl/commit/6984058753cab0fa51cb230117143e2d00bb13ee)) - Karl Hans Laurits
- (**mise**) Move task tools into shared declarations - ([aa4aac5](https://github.com/lauritsk/hatchctl/commit/aa4aac563f8f30b2b8dd6c88d684e9d2bea5fee1)) - Karl Hans Laurits
- (**mise**) Make lint aggregate actionlint checks - ([b4980d4](https://github.com/lauritsk/hatchctl/commit/b4980d4cbc729504f71478fa473efd7cf23aecb1)) - Karl Hans Laurits
- (**mise**) Update hatchctl tool version - ([9621cb9](https://github.com/lauritsk/hatchctl/commit/9621cb901dec10c92b114a20dd9cac630644d393)) - Karl Hans Laurits

- - -

## [v0.6.12](https://github.com/lauritsk/hatchctl/compare/87c9d5129661b3b6a94424baef2bfba43a345bd1..v0.6.12) - 2026-04-10
#### Bug Fixes
- (**bridge**) Preserve exact localhost callback ports for OAuth flows - ([f5459be](https://github.com/lauritsk/hatchctl/commit/f5459be820dc5cdf36707367915673791feec617)) - Karl Hans Laurits
- (**release**) Generate archive SBOMs explicitly - ([b08de67](https://github.com/lauritsk/hatchctl/commit/b08de67c3f02b07a1065b890d515433fa751b392)) - Karl Hans Laurits
#### Documentation
- remove unnecessary security audit doc - ([87c9d51](https://github.com/lauritsk/hatchctl/commit/87c9d5129661b3b6a94424baef2bfba43a345bd1)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([b8bcb47](https://github.com/lauritsk/hatchctl/commit/b8bcb47498e654fa3311bcd25e79d5c08b295eee)) - Karl Hans Laurits

- - -

## [v0.6.11](https://github.com/lauritsk/hatchctl/compare/948295271fe7457ea24dc71846fb63838463c65a..v0.6.11) - 2026-04-10
#### Bug Fixes
- (**release**) Remove git tag section from goreleaser config - ([9482952](https://github.com/lauritsk/hatchctl/commit/948295271fe7457ea24dc71846fb63838463c65a)) - Karl Hans Laurits

- - -

## [v0.6.10](https://github.com/lauritsk/hatchctl/compare/eb79d3ec038a933b03a1baa4a28ad06af19d2249..v0.6.10) - 2026-04-10
#### Bug Fixes
- (**security**) Reduce implicit workspace and bridge trust - ([b5c9943](https://github.com/lauritsk/hatchctl/commit/b5c9943ab43b4fb334262ec4f27ef733fe6a55bc)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Configure Syft for release publishing - ([cb43fc3](https://github.com/lauritsk/hatchctl/commit/cb43fc39338249ce633aef06badd09543875b67d)) - Karl Hans Laurits
- (**release**) Avoid lexicographic tag ordering issues - ([41236c5](https://github.com/lauritsk/hatchctl/commit/41236c59aa36c7435cfa061dab2a75ea08289662)) - Karl Hans Laurits
- (**release**) Report release artifact sizes - ([03a4f3e](https://github.com/lauritsk/hatchctl/commit/03a4f3e3b84a177a1ba3607003d20da9fb55eaf2)) - Karl Hans Laurits
- Enable Renovate platform automerge - ([eb79d3e](https://github.com/lauritsk/hatchctl/commit/eb79d3ec038a933b03a1baa4a28ad06af19d2249)) - Karl Hans Laurits

- - -

## [v0.6.9](https://github.com/lauritsk/hatchctl/compare/5d3165b1ff67b353a93b1a49a43d61dc17495e95..v0.6.9) - 2026-04-10
#### Bug Fixes
- (**deps**) Keep dependency upgrades mergeable with Charm v2 imports and refreshed Alpine fixtures - ([d55f97a](https://github.com/lauritsk/hatchctl/commit/d55f97acfc504278122d6588fce8518c1b687660)) - Karl Hans Laurits
- (**deps**) update github.com/tailscale/hujson digest to ecc657c (#8) - ([8db7ba2](https://github.com/lauritsk/hatchctl/commit/8db7ba23bae85e640f323eacb535be0379c022dd)) - renovate[bot], renovate[bot]
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([5d3165b](https://github.com/lauritsk/hatchctl/commit/5d3165b1ff67b353a93b1a49a43d61dc17495e95)) - Karl Hans Laurits
- Let Renovate manage dependency updates - ([d8c6ba5](https://github.com/lauritsk/hatchctl/commit/d8c6ba595b693692a70def512990eee562a5d657)) - Karl Hans Laurits
- Add Renovate configuration - ([6b407d8](https://github.com/lauritsk/hatchctl/commit/6b407d8d1fee702113992c3c3acd33b4bd94e403)) - Karl Hans Laurits
- Strengthen runtime guardrails and release verification - ([0b4d34f](https://github.com/lauritsk/hatchctl/commit/0b4d34ff8daea0379c60f597292690046a899dfb)) - Karl Hans Laurits

- - -

## [v0.6.8](https://github.com/lauritsk/hatchctl/compare/2df92c03eca16d23f588e11f9e7bfdc269527967..v0.6.8) - 2026-04-10
#### Bug Fixes
- (**runtime**) Persist workspace trust between up and exec - ([ba0df58](https://github.com/lauritsk/hatchctl/commit/ba0df581d206d7b19657ca8ba2839fb02f8bb247)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([2df92c0](https://github.com/lauritsk/hatchctl/commit/2df92c03eca16d23f588e11f9e7bfdc269527967)) - Karl Hans Laurits

- - -

## [v0.6.7](https://github.com/lauritsk/hatchctl/compare/d162e2fa39be7c18940437ce3dfdc0e4267bb294..v0.6.7) - 2026-04-09
#### Bug Fixes
- (**repo**) Surface cache warnings and verify supported runtimes - ([d162e2f](https://github.com/lauritsk/hatchctl/commit/d162e2fa39be7c18940437ce3dfdc0e4267bb294)) - Karl Hans Laurits
- (**runtime**) Pull source images before feature metadata resolution - ([7b28d91](https://github.com/lauritsk/hatchctl/commit/7b28d912f512bf0a1bdedbb034bf79b8387d262b)) - Karl Hans Laurits

- - -

## [v0.6.6](https://github.com/lauritsk/hatchctl/compare/624e7b9e43fa9eb3429547ae8c0aadea1c08f72f..v0.6.6) - 2026-04-09
#### Bug Fixes
- (**runtime**) Restore source-image users and dotfiles on recreate - ([624e7b9](https://github.com/lauritsk/hatchctl/commit/624e7b9e43fa9eb3429547ae8c0aadea1c08f72f)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Bump hatchctl tool to v0.6.5 - ([930dd2a](https://github.com/lauritsk/hatchctl/commit/930dd2a4258f670459d52c39f8d1e3af758861b0)) - Karl Hans Laurits

- - -

## [v0.6.5](https://github.com/lauritsk/hatchctl/compare/edbc1b88d66365c2342b2c4048910517390f7335..v0.6.5) - 2026-04-09
#### Bug Fixes
- (**runtime**) Use managed runtime metadata for exec and dotfiles - ([2d59e16](https://github.com/lauritsk/hatchctl/commit/2d59e16d6e93f402be6d80bde3f6b369307897b7)) - Karl Hans Laurits
#### Refactoring
- (**capability**) Extract runtime capabilities into dedicated packages - ([ce0ff7e](https://github.com/lauritsk/hatchctl/commit/ce0ff7e8cc4b30f01fb10f042bb4a9c062c8214e)) - Karl Hans Laurits
- (**engine**) Route runtime exec flows through typed engine requests - ([5afee49](https://github.com/lauritsk/hatchctl/commit/5afee49755f6b5b431c264c2c8dbc02bf985b5ce)) - Karl Hans Laurits
- (**plan**) Introduce workspace plans for runtime flows - ([4001930](https://github.com/lauritsk/hatchctl/commit/40019305ce1dc05bb6d92053558a208371063fba)) - Karl Hans Laurits
- (**reconcile**) Add first image and container reconciler - ([405dd7e](https://github.com/lauritsk/hatchctl/commit/405dd7e18dd0fcfdc46c094494c3e050297c09b6)) - Karl Hans Laurits
- (**runtime**) Finish the plan observe reconcile migration - ([812384d](https://github.com/lauritsk/hatchctl/commit/812384d5cb55f813af4e52b2867cf601175bebef)) - Karl Hans Laurits
- (**runtime**) Move shipped orchestration and persistence out of runtime - ([b1b8bb1](https://github.com/lauritsk/hatchctl/commit/b1b8bb1b92deaee3eedb4795d1aa137f90ea6c7b)) - Karl Hans Laurits
- (**runtime**) Reduce runtime runner to a thin command facade - ([6054276](https://github.com/lauritsk/hatchctl/commit/60542761ea0e0b731de1bb33477dcdbd305fddc6)) - Karl Hans Laurits
- (**runtime**) Move exec and lifecycle onto observed targets - ([88252f8](https://github.com/lauritsk/hatchctl/commit/88252f8d3be24844fa44003ae09903169f6978d0)) - Karl Hans Laurits
- (**runtime**) Add engine and observation seams for runtime actions - ([36900f2](https://github.com/lauritsk/hatchctl/commit/36900f231268a6eacb71141a5bfbffa02cf875c3)) - Karl Hans Laurits
- (**runtime**) Prepare the refactor with app, policy, and store seams - ([b91131c](https://github.com/lauritsk/hatchctl/commit/b91131c686f22d350fedf26d11bea5cba8d2c8b8)) - Karl Hans Laurits
- (**spec**) Extract pure devcontainer spec loading - ([35b53b0](https://github.com/lauritsk/hatchctl/commit/35b53b041d4e3df6c283ac342b3daca13715c084)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**deps**) bump github.com/BurntSushi/toml from 1.5.0 to 1.6.0 - ([bf3aa19](https://github.com/lauritsk/hatchctl/commit/bf3aa19f36f892c1cdce38de00ae777b8e4949f7)) - dependabot[bot]
- (**mise**) Update hatchctl tool - ([edbc1b8](https://github.com/lauritsk/hatchctl/commit/edbc1b88d66365c2342b2c4048910517390f7335)) - Karl Hans Laurits

- - -

## [v0.6.4](https://github.com/lauritsk/hatchctl/compare/9eacadfce9ebc22255a4b52d33ac1cd185e5c7d3..v0.6.4) - 2026-04-09
#### Bug Fixes
- (**docker**) Keep build progress visible after trust prompts - ([9eacadf](https://github.com/lauritsk/hatchctl/commit/9eacadfce9ebc22255a4b52d33ac1cd185e5c7d3)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool to v0.6.3 - ([96cae57](https://github.com/lauritsk/hatchctl/commit/96cae5761f33672f550e250091b22d51e216c834)) - Karl Hans Laurits

- - -

## [v0.6.3](https://github.com/lauritsk/hatchctl/compare/ad0d9cac12d3ba2e35b99102fc7c638e82bf432b..v0.6.3) - 2026-04-09
#### Bug Fixes
- (**runtime**) Preserve macOS SSH forwarding and feature runtime metadata - ([3248e38](https://github.com/lauritsk/hatchctl/commit/3248e38ac9219b0e91cf98f56ac1d50adc03edc4)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool to v0.6.2 - ([ad0d9ca](https://github.com/lauritsk/hatchctl/commit/ad0d9cac12d3ba2e35b99102fc7c638e82bf432b)) - Karl Hans Laurits

- - -

## [v0.6.2](https://github.com/lauritsk/hatchctl/compare/fcaa81d87c54d69e96664bc873ca1dd1dc27f8c2..v0.6.2) - 2026-04-09
#### Bug Fixes
- (**runtime**) Preserve prompt rendering and SSH agent mounts - ([9ab5c9a](https://github.com/lauritsk/hatchctl/commit/9ab5c9a53160b5922ae603e7e9bea8d0b05a8b8f)) - Karl Hans Laurits
- Promote BurntSushi/toml to a direct dependency - ([fcaa81d](https://github.com/lauritsk/hatchctl/commit/fcaa81d87c54d69e96664bc873ca1dd1dc27f8c2)) - Karl Hans Laurits
#### Miscellaneous Chores
- Update hatchctl tool to v0.6.1 - ([8dd4d5b](https://github.com/lauritsk/hatchctl/commit/8dd4d5beef8a86b5a58706e1ea5bd7844467aef1)) - Karl Hans Laurits

- - -

## [v0.6.1](https://github.com/lauritsk/hatchctl/compare/cfa13d8fa5cc2e9d62416908a4a5258e363b1fdb..v0.6.1) - 2026-04-09
#### Bug Fixes
- (**runtime**) Avoid closing live descriptors during TTY detection - ([82ec2ae](https://github.com/lauritsk/hatchctl/commit/82ec2ae25c23b03f4e663776154da66b118cb5bb)) - Karl Hans Laurits
- (**runtime**) Stabilize atomic state writes in CI - ([298f109](https://github.com/lauritsk/hatchctl/commit/298f109ee8ea0bfa3dbe43219b41c55d7405c177)) - Karl Hans Laurits
- (**runtime**) Unblock integration feature resolution - ([d4d4c97](https://github.com/lauritsk/hatchctl/commit/d4d4c97bc6714437c924d115a8dc9195d11b7eca)) - Karl Hans Laurits
- (**runtime**) Use a static bridge helper stub in runtime tests - ([6a4c7e8](https://github.com/lauritsk/hatchctl/commit/6a4c7e8bc75adc7be82dafbc8163151539d9c909)) - Karl Hans Laurits
- (**runtime**) Remove shared bridge helper file cache in tests - ([86b12ab](https://github.com/lauritsk/hatchctl/commit/86b12ab3551bf6a20100b9ace72853f1c0617fe5)) - Karl Hans Laurits
- (**runtime**) Avoid flaky bridge helper copy in runtime integration tests - ([6e0213a](https://github.com/lauritsk/hatchctl/commit/6e0213aeb332a831d23180fb8d52653a711a45f7)) - Karl Hans Laurits
#### Tests
- (**runtime**) Decouple runtime orchestration tests from shell fakes - ([6e61d4a](https://github.com/lauritsk/hatchctl/commit/6e61d4a9a86b9b4d127f32d6cf3939d410710832)) - Karl Hans Laurits
- Split integration suites and share test harnesses - ([b8d34ba](https://github.com/lauritsk/hatchctl/commit/b8d34ba4b6d98104fc6b0e7f04f7b799cf80ace7)) - Karl Hans Laurits
- Shorten the default test feedback loop - ([bff220a](https://github.com/lauritsk/hatchctl/commit/bff220a351c28ba375d417755828e755e9e0c406)) - Karl Hans Laurits
- Reduce brittle Go test coverage - ([cfa13d8](https://github.com/lauritsk/hatchctl/commit/cfa13d8fa5cc2e9d62416908a4a5258e363b1fdb)) - Karl Hans Laurits
#### Continuous Integration
- Reuse Go caches across GitHub Actions runs - ([d774c33](https://github.com/lauritsk/hatchctl/commit/d774c33c9893615c3d057ef14f4ab12d8b75a911)) - Karl Hans Laurits
#### Refactoring
- Consolidate workspace orchestration and harden feature resolution - ([b563fc2](https://github.com/lauritsk/hatchctl/commit/b563fc2fe8c8043255f6b00e09d9d013d53c6f96)) - Karl Hans Laurits

- - -

## [v0.6.0](https://github.com/lauritsk/hatchctl/compare/e3f08b32833d8b43f6295e63d37521cd19979f48..v0.6.0) - 2026-04-08
#### Features
- (**runtime**) Add ssh-agent passthrough for managed containers - ([e3f08b3](https://github.com/lauritsk/hatchctl/commit/e3f08b32833d8b43f6295e63d37521cd19979f48)) - Karl Hans Laurits
#### Bug Fixes
- (**runtime**) Stabilize runtime Docker integrations on Darwin - ([e7aa15a](https://github.com/lauritsk/hatchctl/commit/e7aa15a9702e723f15d8a1341f56693913db43ee)) - Karl Hans Laurits
- (**runtime**) Avoid copying image verification policy locks - ([fa0299f](https://github.com/lauritsk/hatchctl/commit/fa0299f9c3bb8d5cdff6326d239423302e505fe6)) - Karl Hans Laurits
#### Miscellaneous Chores
- Track Darwin integration stability follow-up - ([59f850a](https://github.com/lauritsk/hatchctl/commit/59f850abb15cc2bbbec4c6c8b05ee6da62c2c2f9)) - Karl Hans Laurits
- Improve CLI guidance and dependency update defaults - ([c39ce95](https://github.com/lauritsk/hatchctl/commit/c39ce95721eb99474b30216ba70fd927dd2fc6c7)) - Karl Hans Laurits

- - -

## [v0.5.1](https://github.com/lauritsk/hatchctl/compare/3ab1fa875bfff4a591e963c231101378fa3bfd12..v0.5.1) - 2026-04-08
#### Bug Fixes
- (**runtime**) Harden untrusted workspace and artifact trust checks - ([3ab1fa8](https://github.com/lauritsk/hatchctl/commit/3ab1fa875bfff4a591e963c231101378fa3bfd12)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([8d91533](https://github.com/lauritsk/hatchctl/commit/8d915335305ca211c05c1a147523d88661baf2c3)) - Karl Hans Laurits

- - -

## [v0.5.0](https://github.com/lauritsk/hatchctl/compare/35ff158842fb2509ce86fa65c58fec49588b63db..v0.5.0) - 2026-04-08
#### Features
- (**runtime**) Add structured runtime phases to progress output - ([35ff158](https://github.com/lauritsk/hatchctl/commit/35ff158842fb2509ce86fa65c58fec49588b63db)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([db01648](https://github.com/lauritsk/hatchctl/commit/db016482399375beb3a2281605827da42797625d)) - Karl Hans Laurits

- - -

## [v0.4.0](https://github.com/lauritsk/hatchctl/compare/e0c2f22da0817b9ef0ea3cce60009e73d90feb6a..v0.4.0) - 2026-04-08
#### Features
- (**cli**) Improve inline terminal summaries - ([e0c2f22](https://github.com/lauritsk/hatchctl/commit/e0c2f22da0817b9ef0ea3cce60009e73d90feb6a)) - Karl Hans Laurits
#### Bug Fixes
- (**cli**) Clarify CLI error wording and display - ([de052ff](https://github.com/lauritsk/hatchctl/commit/de052ffbd521cc3c20998efcbc3cb0e09643e344)) - Karl Hans Laurits
- (**runtime**) Support chezmoi-style dotfiles repo guessing - ([d0a9165](https://github.com/lauritsk/hatchctl/commit/d0a91655720c6eccea4ef531542f761d34528d79)) - Karl Hans Laurits

- - -

## [v0.3.6](https://github.com/lauritsk/hatchctl/compare/e092ae72c14ff50a6b1c4320e375f8da9d31edef..v0.3.6) - 2026-04-08
#### Bug Fixes
- (**cli**) Preserve terminal access for interactive exec tools - ([9e7e81a](https://github.com/lauritsk/hatchctl/commit/9e7e81a15d1d00195a2899630766305a5ddc362c)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([e092ae7](https://github.com/lauritsk/hatchctl/commit/e092ae72c14ff50a6b1c4320e375f8da9d31edef)) - Karl Hans Laurits

- - -

## [v0.3.5](https://github.com/lauritsk/hatchctl/compare/03a85dfe35b3b185c768034597144688962fc2a0..v0.3.5) - 2026-04-08
#### Bug Fixes
- (**bridge**) Preserve loopback host in bridge auth callbacks - ([03f8bd8](https://github.com/lauritsk/hatchctl/commit/03f8bd871b6aec05baa265c02d4ac657625fe5c9)) - Karl Hans Laurits
- (**runtime**) Start exec in the workspace shell by default - ([03a85df](https://github.com/lauritsk/hatchctl/commit/03a85dfe35b3b185c768034597144688962fc2a0)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version - ([7d68629](https://github.com/lauritsk/hatchctl/commit/7d68629a8bc5af1b1268b6de12aac4541d6945cc)) - Karl Hans Laurits

- - -

## [v0.3.4](https://github.com/lauritsk/hatchctl/compare/00a3a29cffcc21090fbab4f1910d2e47a5ea42b9..v0.3.4) - 2026-04-08
#### Bug Fixes
- (**bridge**) Rewrite embedded callback URLs for browser auth - ([00a3a29](https://github.com/lauritsk/hatchctl/commit/00a3a29cffcc21090fbab4f1910d2e47a5ea42b9)) - Karl Hans Laurits

- - -

## [v0.3.3](https://github.com/lauritsk/hatchctl/compare/9ab8bf06e484f3e378d770d70d7c8c52b5e04193..v0.3.3) - 2026-04-08
#### Bug Fixes
- (**runtime**) Preserve base image metadata in feature images - ([8824d48](https://github.com/lauritsk/hatchctl/commit/8824d484a74fb161b26d6b8ebe29694c681b93b0)) - Karl Hans Laurits
- (**security**) harden workspace trust boundaries - ([9ab8bf0](https://github.com/lauritsk/hatchctl/commit/9ab8bf06e484f3e378d770d70d7c8c52b5e04193)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) Update hatchctl tool version and document commit task usage - ([ff833a3](https://github.com/lauritsk/hatchctl/commit/ff833a3b9343f0e016b8d4352cc8d0bc000e91c7)) - Karl Hans Laurits

- - -

## [v0.3.2](https://github.com/lauritsk/hatchctl/compare/449ae146a5724b4ffad54360366346be8d03fc42..v0.3.2) - 2026-04-08
#### Bug Fixes
- (**runtime**) resolve dotfiles home and restore interactive exec tty - ([30bf1f3](https://github.com/lauritsk/hatchctl/commit/30bf1f31f7fa24394f8d44f5470c759d4449c27d)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**mise**) add hatchctl tool dependency - ([449ae14](https://github.com/lauritsk/hatchctl/commit/449ae146a5724b4ffad54360366346be8d03fc42)) - Karl Hans Laurits

- - -

## [v0.3.1](https://github.com/lauritsk/hatchctl/compare/1018af8270ac713d298bb28f472b42d04cf7e322..v0.3.1) - 2026-04-08
#### Bug Fixes
- (**ci**) isolate config-sensitive cli tests - ([bf1d6c7](https://github.com/lauritsk/hatchctl/commit/bf1d6c77ff96cad69c854dec1a51e2b23d3d419b)) - Karl Hans Laurits
#### Refactoring
- (**runtime**) simplify orchestration boundaries - ([1018af8](https://github.com/lauritsk/hatchctl/commit/1018af8270ac713d298bb28f472b42d04cf7e322)) - Karl Hans Laurits

- - -

## [v0.3.0](https://github.com/lauritsk/hatchctl/compare/83637697ca48684cd6aa7d1d6c53bb0dcfed9107..v0.3.0) - 2026-04-08
#### Features
- (**cli**) clarify command UX and runtime copy - ([90caa21](https://github.com/lauritsk/hatchctl/commit/90caa21249de10f766319166572ffb23f3108221)) - Karl Hans Laurits
- (**config**) add config.toml defaults and platform paths - ([c11ac5d](https://github.com/lauritsk/hatchctl/commit/c11ac5d0d98f80a35fe4b62ad9de20714a31b82a)) - Karl Hans Laurits
#### Bug Fixes
- (**config**) make appconfig test honor platform user config paths - ([8f5536b](https://github.com/lauritsk/hatchctl/commit/8f5536b3868758b61b8efc1eba2b4a8383f53920)) - Karl Hans Laurits
#### Documentation
- (**agents**) prefer mise tasks for workflows - ([4bf6f5b](https://github.com/lauritsk/hatchctl/commit/4bf6f5b3f7c39e9d425e634bdac287b980561c2b)) - Karl Hans Laurits
- (**agents**) allow architectural rewrites - ([96743e0](https://github.com/lauritsk/hatchctl/commit/96743e031c6f45e756a2c6400d8b8bc4242f1a81)) - Karl Hans Laurits
- (**readme**) refine project tagline - ([e790626](https://github.com/lauritsk/hatchctl/commit/e790626837e604ab9420da9ccb79a659a57aca07)) - Karl Hans Laurits
- (**readme**) switch install instructions to mise - ([78292d0](https://github.com/lauritsk/hatchctl/commit/78292d0cb75c8a32c083f63559384811bb8473c5)) - Karl Hans Laurits
- (**todo**) mark completed architecture items done - ([7186e51](https://github.com/lauritsk/hatchctl/commit/7186e51a1f164b71d078f4db2a662f4c1667f949)) - Karl Hans Laurits
- (**todo**) refresh remaining roadmap items - ([d5fe848](https://github.com/lauritsk/hatchctl/commit/d5fe8480ec1e61c6dd0181f1530020b9e6f4d2a0)) - Karl Hans Laurits
- (**todo**) add display layer refactor item - ([d6b184c](https://github.com/lauritsk/hatchctl/commit/d6b184c73d84de532ddf3b029effbf115c402857)) - Karl Hans Laurits
#### Tests
- (**runtime**) disable dotfiles integration coverage in CI - ([d851cde](https://github.com/lauritsk/hatchctl/commit/d851cde098eba2d5a14641a0c33eeedbc60d578f)) - Karl Hans Laurits
- (**runtime**) re-enable dotfiles integration fixture to avoid CI safe.directory failures - ([548285a](https://github.com/lauritsk/hatchctl/commit/548285ace3ddc34292a0b452a9cefd48388e27ad)) - Karl Hans Laurits
- (**runtime**) reuse integration fixtures - ([b9d926f](https://github.com/lauritsk/hatchctl/commit/b9d926ff2d6071613db819635b1cb27238ecd435)) - Karl Hans Laurits
#### Build system
- (**mise**) move bridge helper workflow into tasks - ([b87db81](https://github.com/lauritsk/hatchctl/commit/b87db8183261d9e4ac5902a1fc45916728f07ad2)) - Karl Hans Laurits
#### Refactoring
- (**bridge**) isolate bridge planning from runtime - ([2c41665](https://github.com/lauritsk/hatchctl/commit/2c41665894c61015e8d3eda1edb2624d19a39610)) - Karl Hans Laurits
- (**devcontainer**) extract resolver persistence helper - ([7af4f48](https://github.com/lauritsk/hatchctl/commit/7af4f484cae8644bf99516a44f798d72a547b8b6)) - Karl Hans Laurits
- (**devcontainer**) make resolve persistence explicit - ([524296d](https://github.com/lauritsk/hatchctl/commit/524296d6241ebd86c7acc1a6485dc32c09a58326)) - Karl Hans Laurits
- (**display**) centralize runtime output in display - ([7685630](https://github.com/lauritsk/hatchctl/commit/7685630229d990bb12dd2936b847438604db2236)) - Karl Hans Laurits
- (**fileutil**) centralize atomic writes for control files - ([0353f25](https://github.com/lauritsk/hatchctl/commit/0353f25a26ed40b51aa580acb1c88cd2a854d94c)) - Karl Hans Laurits
- (**runtime**) replace assembled runtime shell helpers - ([49fae68](https://github.com/lauritsk/hatchctl/commit/49fae68de286ccdb4ab8781db1285e0f887ca623)) - Karl Hans Laurits
- (**runtime**) narrow runtime command execution seam - ([6932ebe](https://github.com/lauritsk/hatchctl/commit/6932ebeb78dbe07f5099c246d3a959a0488882c5)) - Karl Hans Laurits
- (**runtime**) route runtime warnings and progress output through display events - ([878cd6c](https://github.com/lauritsk/hatchctl/commit/878cd6c9e6f9be6855dbc2ea7dc5ea20cccf9107)) - Karl Hans Laurits
- (**runtime**) centralize command execution backend - ([265544a](https://github.com/lauritsk/hatchctl/commit/265544ab66a1630ee37cb91a9dc5f2be3f05a100)) - Karl Hans Laurits
- (**runtime**) add engine adapter - ([3cefd52](https://github.com/lauritsk/hatchctl/commit/3cefd52c33c3bfe9302b4a1e6f4224a8642b211d)) - Karl Hans Laurits
- (**runtime**) centralize process execution - ([6a40c2b](https://github.com/lauritsk/hatchctl/commit/6a40c2be7c3e66c70c78152ac0236629023ac9e9)) - Karl Hans Laurits
- (**runtime**) inject resolver and state store - ([ca1baa9](https://github.com/lauritsk/hatchctl/commit/ca1baa92a5859ac9381b9ecc64869f206eb87beb)) - Karl Hans Laurits
- (**runtime**) extract lifecycle manager - ([c814b7e](https://github.com/lauritsk/hatchctl/commit/c814b7e5bc2619a7b771468a1f441c3ad9d92ef6)) - Karl Hans Laurits
- (**runtime**) extract image and container managers - ([73db225](https://github.com/lauritsk/hatchctl/commit/73db225a3a89e14abd5937bfe63ed252cd73cd50)) - Karl Hans Laurits
- (**runtime**) extract planning collaborators - ([8363769](https://github.com/lauritsk/hatchctl/commit/83637697ca48684cd6aa7d1d6c53bb0dcfed9107)) - Karl Hans Laurits
- (**security**) move trust policy to runtime boundary - ([82d5691](https://github.com/lauritsk/hatchctl/commit/82d5691d4cd982a0935f9b633065af9c75205f63)) - Karl Hans Laurits
- (**security**) move verification policy to runtime - ([7b857ad](https://github.com/lauritsk/hatchctl/commit/7b857adf163cf97af27c547e9ee2cbd8599301f7)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**repo**) simplify agent workflow instructions - ([6fb010a](https://github.com/lauritsk/hatchctl/commit/6fb010a37c2ef251b7fb38247ff86f6abfecb9b4)) - Karl Hans Laurits
- (**repo**) clarify agent workflow instructions - ([4b5f816](https://github.com/lauritsk/hatchctl/commit/4b5f816330c59e05f396001d709f30655044d7a4)) - Karl Hans Laurits
- (**repo**) require commit and push after verified completion - ([5c3821e](https://github.com/lauritsk/hatchctl/commit/5c3821e7aae703de591ff6f9543f25fd9843b67c)) - Karl Hans Laurits
- (**repo**) use mise task for cocogitto commits - ([88073ca](https://github.com/lauritsk/hatchctl/commit/88073cae184d0549a000bec7b8782a2a07011bee)) - Karl Hans Laurits

- - -

## [v0.2.3](https://github.com/lauritsk/hatchctl/compare/a2453c20794cea2e0e5baa61d7570b7f5b0f8306..v0.2.3) - 2026-04-08
#### Bug Fixes
- (**runtime**) resolve HOME for exec user - ([0a1b16c](https://github.com/lauritsk/hatchctl/commit/0a1b16cee70937bc426510d299025c75ea406ad2)) - Karl Hans Laurits
- (**ui**) separate progress spinners from streamed logs - ([e7373f8](https://github.com/lauritsk/hatchctl/commit/e7373f85d69808fb365013ac49bee5553dfd0bd6)) - Karl Hans Laurits
- (**up**) suggest follow-up exec commands - ([a2453c2](https://github.com/lauritsk/hatchctl/commit/a2453c20794cea2e0e5baa61d7570b7f5b0f8306)) - Karl Hans Laurits
#### Documentation
- (**todo**) add prioritized architecture backlog - ([988e814](https://github.com/lauritsk/hatchctl/commit/988e8149c5ecefd51ac5c35cd1f4203ee58adbfe)) - Karl Hans Laurits
- (**todo**) track config and artifact location work - ([f086951](https://github.com/lauritsk/hatchctl/commit/f086951e72b4029431e9640877bf6cdd37c44c00)) - Karl Hans Laurits

- - -

## [v0.2.2](https://github.com/lauritsk/hatchctl/compare/3b7fb715b1c91c3181ead7ed2235f48b4ffb5feb..v0.2.2) - 2026-04-08
#### Bug Fixes
- (**ci**) skip flaky dotfiles integration test - ([3b7fb71](https://github.com/lauritsk/hatchctl/commit/3b7fb715b1c91c3181ead7ed2235f48b4ffb5feb)) - Karl Hans Laurits
- (**exec**) clear spinner before interactive handoff - ([7857016](https://github.com/lauritsk/hatchctl/commit/7857016cbc2c1a0491e5aaf3573cbc196ce3c9be)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**git**) ignore local build output - ([fcff2ab](https://github.com/lauritsk/hatchctl/commit/fcff2abe6bcd75cca57de43c4aaf6a7c64603578)) - Karl Hans Laurits

- - -

## [v0.2.1](https://github.com/lauritsk/hatchctl/compare/314b223eff8c1b7c60b29cbffeb0ff8807c71110..v0.2.1) - 2026-04-08
#### Bug Fixes
- (**bridge**) remove stale unix socket paths - ([314b223](https://github.com/lauritsk/hatchctl/commit/314b223eff8c1b7c60b29cbffeb0ff8807c71110)) - Karl Hans Laurits
- (**runtime**) disable signing in test git commits - ([2ffdf22](https://github.com/lauritsk/hatchctl/commit/2ffdf223ade14335c97ff0a0c4080c0940bb80bd)) - Karl Hans Laurits

- - -

## [v0.2.0](https://github.com/lauritsk/hatchctl/compare/97d3761d8083cab0c0a23bd2e84ed734b3d82ae5..v0.2.0) - 2026-04-08
#### Features
- (**runtime**) add dotfiles personalization support - ([97d3761](https://github.com/lauritsk/hatchctl/commit/97d3761d8083cab0c0a23bd2e84ed734b3d82ae5)) - Karl Hans Laurits
#### Bug Fixes
- (**bridge**) use portable bridge transports - ([6809f80](https://github.com/lauritsk/hatchctl/commit/6809f803933908b8cadd022bb974f8e039d8d16a)) - Karl Hans Laurits
- (**runtime**) make dotfiles test self-contained - ([c1da74c](https://github.com/lauritsk/hatchctl/commit/c1da74cc183405b7268d4ba1b77b0ff7d4029b06)) - Karl Hans Laurits

- - -

## [v0.1.6](https://github.com/lauritsk/hatchctl/compare/7ee076f237d2fba8edc70090308547b88e8a19b7..v0.1.6) - 2026-04-08
#### Bug Fixes
- (**display**) stabilize spinner cursor rendering - ([7ee076f](https://github.com/lauritsk/hatchctl/commit/7ee076f237d2fba8edc70090308547b88e8a19b7)) - Karl Hans Laurits
- (**runtime**) inline feature build base image - ([0d265e0](https://github.com/lauritsk/hatchctl/commit/0d265e0c5a5d55b848e08720d467dd4e88b56cfa)) - Karl Hans Laurits
- (**runtime**) reduce bridge startup flakes and stop delays - ([d086dbe](https://github.com/lauritsk/hatchctl/commit/d086dbeeead0f5fd4d433a45135ac8228064ff26)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**devcontainer**) add local hatchctl setup - ([891f5a4](https://github.com/lauritsk/hatchctl/commit/891f5a478c4be9557e95314e7dfe162fac6a7bda)) - Karl Hans Laurits

- - -

## [v0.1.5](https://github.com/lauritsk/hatchctl/compare/b890004b4bf5c25f6dd916b56e638ef84331b3ca..v0.1.5) - 2026-04-08
#### Bug Fixes
- (**release**) generate embedded bridge helpers during release - ([e23dbf8](https://github.com/lauritsk/hatchctl/commit/e23dbf8a66046b544e15cd924342fe5eee2af19f)) - Karl Hans Laurits
- (**release**) let goreleaser autodetect binaries - ([b890004](https://github.com/lauritsk/hatchctl/commit/b890004b4bf5c25f6dd916b56e638ef84331b3ca)) - Karl Hans Laurits
- (**runtime**) harden container and feature resolution - ([83b3839](https://github.com/lauritsk/hatchctl/commit/83b38393aa6a3a8a11250afed5341d1a09435d1b)) - Karl Hans Laurits

- - -

## [v0.1.4](https://github.com/lauritsk/hatchctl/compare/bbd7dd84a3da79180cea9fe191d7f55646dba966..v0.1.4) - 2026-04-08
#### Bug Fixes
- (**bridge**) make --bridge work without extra setup - ([d25afa8](https://github.com/lauritsk/hatchctl/commit/d25afa8167c5d345ff0e6032b4f3082bea835fce)) - Karl Hans Laurits
#### Performance Improvements
- (**devcontainer**) cache resolved workspace plans - ([101bc0b](https://github.com/lauritsk/hatchctl/commit/101bc0bdec47408b9d6a03668ea9051bfd0c0f70)) - Karl Hans Laurits
#### Tests
- (**runtime**) cover CLI and bridge edge cases - ([e75bc5e](https://github.com/lauritsk/hatchctl/commit/e75bc5e478bd9b0d56492af70c3a27317721f554)) - Karl Hans Laurits
#### Refactoring
- (**release**) align versioning and bridge packaging - ([30baa1f](https://github.com/lauritsk/hatchctl/commit/30baa1f699182f2034747e7840a1755cf6b6a765)) - Karl Hans Laurits
- drop legacy bridge helper packaging - ([a6a66d9](https://github.com/lauritsk/hatchctl/commit/a6a66d97b96975d0a57140652cb2525e44ca2083)) - Karl Hans Laurits
#### Miscellaneous Chores
- (**deps**) bump Go toolchain to 1.26.2 - ([bbd7dd8](https://github.com/lauritsk/hatchctl/commit/bbd7dd84a3da79180cea9fe191d7f55646dba966)) - Karl Hans Laurits
- (**mise**) scope gofumpt to formatting tasks - ([79d3ac9](https://github.com/lauritsk/hatchctl/commit/79d3ac94b0155fa4e355c79834ca7d3d2cf28ddb)) - Karl Hans Laurits
- (**release**) restore goreleaser defaults - ([7aaf8fc](https://github.com/lauritsk/hatchctl/commit/7aaf8fc76da4a799688c3430a5bbd3f59cd8db60)) - Karl Hans Laurits
- align release check goreleaser version - ([8dbb3ed](https://github.com/lauritsk/hatchctl/commit/8dbb3ed6823cb44c095520c139df697d34e4dd1a)) - Karl Hans Laurits

- - -

## [v0.1.3](https://github.com/lauritsk/hatchctl/compare/901883fcf6ec5249a76a170d793c3a09c791da99..v0.1.3) - 2026-04-07
#### Bug Fixes
- embed bridge helpers in release builds - ([2b931ea](https://github.com/lauritsk/hatchctl/commit/2b931ea47141773eb96d2e7b3a2c7c1dea4055a5)) - Karl Hans Laurits
- make feature fetch timeouts configurable - ([901883f](https://github.com/lauritsk/hatchctl/commit/901883fcf6ec5249a76a170d793c3a09c791da99)) - Karl Hans Laurits
#### Refactoring
- move uid updates to runtime - ([dae92fe](https://github.com/lauritsk/hatchctl/commit/dae92feccebf12bb7809136f0ceddedadf223a9d)) - Karl Hans Laurits
- fold bridge helper into hatchctl - ([046ddba](https://github.com/lauritsk/hatchctl/commit/046ddba681cf169781b3c3597a444d27f6ae7777)) - Karl Hans Laurits
- share runtime preparation state - ([9385bee](https://github.com/lauritsk/hatchctl/commit/9385bee755435e496560f77f92d2044374c02346)) - Karl Hans Laurits
- replace bridge HTTP transport - ([c9b0cc2](https://github.com/lauritsk/hatchctl/commit/c9b0cc2d155d912a4458f6a4a365bf456aab1275)) - Karl Hans Laurits
- stop publishing standalone bridge helper - ([eec0666](https://github.com/lauritsk/hatchctl/commit/eec066635d8a047d5146ae87fd2ec3735e525e56)) - Karl Hans Laurits

- - -

## [v0.1.2](https://github.com/lauritsk/hatchctl/compare/85c0692f9a9873d0648efa8eee811d8f8ff79e81..v0.1.2) - 2026-04-07
#### Bug Fixes
- separate goreleaser archive outputs - ([85c0692](https://github.com/lauritsk/hatchctl/commit/85c0692f9a9873d0648efa8eee811d8f8ff79e81)) - Karl Hans Laurits

- - -

## [v0.1.1](https://github.com/lauritsk/hatchctl/compare/c66c83c88fa8eea82ac7ed74c8ebcee6b9eb7ded..v0.1.1) - 2026-04-07
#### Bug Fixes
- repair release workflow authentication - ([c0ee5de](https://github.com/lauritsk/hatchctl/commit/c0ee5de0e38da28f62ba947659587abca725835e)) - Karl Hans Laurits
#### Miscellaneous Chores
- add changelog authoring to Cocogitto configuration - ([c66c83c](https://github.com/lauritsk/hatchctl/commit/c66c83c88fa8eea82ac7ed74c8ebcee6b9eb7ded)) - Karl Hans Laurits

- - -

## [v0.1.0](https://github.com/lauritsk/hatchctl/compare/8d08d1ba8510d10cc98236e0c8fb3b1a684abd22..v0.1.0) - 2026-04-07
#### Features
- improve command progress and debug output - ([de50e4b](https://github.com/lauritsk/hatchctl/commit/de50e4b8627ab480cf5821d872446210de69b31b)) - Karl Hans Laurits
- add feature lockfile policies - ([f079660](https://github.com/lauritsk/hatchctl/commit/f0796607aea706fef41fb9874234bc1e9468770d)) - Karl Hans Laurits
- verify remote images with cosign library - ([ef8b4c1](https://github.com/lauritsk/hatchctl/commit/ef8b4c1d74aa661c1fe7255a2b26ef6ad8397f09)) - Karl Hans Laurits
- add feature lockfile and tarball support - ([dfa25d2](https://github.com/lauritsk/hatchctl/commit/dfa25d29e38a2240a4de850a67a9fdb94d868160)) - Karl Hans Laurits
- add compose bridge and uid parity - ([6b4dbf8](https://github.com/lauritsk/hatchctl/commit/6b4dbf88487e81ce0aad104eba764b94a4aabe09)) - Karl Hans Laurits
- add compose feature support - ([bbffa5e](https://github.com/lauritsk/hatchctl/commit/bbffa5ea02972bd1b27572c4a4f641698684ce94)) - Karl Hans Laurits
- persist compose override files - ([b525acf](https://github.com/lauritsk/hatchctl/commit/b525acf72ff8c84bdf5d947773a2816ce06b7603)) - Karl Hans Laurits
- add oci features and compose support - ([17f3a0d](https://github.com/lauritsk/hatchctl/commit/17f3a0d503a0b403f8cf76185d1191c4d38306de)) - Karl Hans Laurits
- add local single-container feature support - ([1774022](https://github.com/lauritsk/hatchctl/commit/1774022ba9aee2a257dc825151ff508107202255)) - Karl Hans Laurits
- use mounted bridge helper binary - ([a0eca42](https://github.com/lauritsk/hatchctl/commit/a0eca422439c76c6d3b4fe2b10596fece0df007b)) - Karl Hans Laurits
- start host bridge runtime - ([c6f74e0](https://github.com/lauritsk/hatchctl/commit/c6f74e0a1764e6d7b1783eaee6c4a67941da6889)) - Karl Hans Laurits
- mount bridge helper binaries - ([79f1f00](https://github.com/lauritsk/hatchctl/commit/79f1f00302bf871a8f748d37c57f9b4bc736b764)) - Karl Hans Laurits
- reconcile managed container state - ([23029a7](https://github.com/lauritsk/hatchctl/commit/23029a7566584b9dbf48da13310d4bce6f3cc028)) - Karl Hans Laurits
- update non-root user uid and gid - ([f9895a3](https://github.com/lauritsk/hatchctl/commit/f9895a3cb0d04d0087dec6c69e65615b77f5e715)) - Karl Hans Laurits
- tighten exec runtime behavior - ([d1d106d](https://github.com/lauritsk/hatchctl/commit/d1d106d4941940f8bf616190fba40f4e53b6ee3a)) - Karl Hans Laurits
- refine remote user fallback - ([f765429](https://github.com/lauritsk/hatchctl/commit/f7654292f254aaac59d4e8c451850f5fc961ef5a)) - Karl Hans Laurits
- inspect managed container config state - ([ec36ea3](https://github.com/lauritsk/hatchctl/commit/ec36ea33284d80cd7bd0844ccba69d0b41cfd58d)) - Karl Hans Laurits
- wire forward ports into merged runtime config - ([26b519e](https://github.com/lauritsk/hatchctl/commit/26b519e17ad8790cfb6219c405a35d22c83cc7f9)) - Karl Hans Laurits
- persist devcontainer metadata labels - ([deeba0d](https://github.com/lauritsk/hatchctl/commit/deeba0d71f5a14fe06e5c8207652373e775957f5)) - Karl Hans Laurits
- merge image metadata into runtime config - ([5f8be5e](https://github.com/lauritsk/hatchctl/commit/5f8be5e96945762b741c75e706da73bd4c46a7ed)) - Karl Hans Laurits
- bootstrap single-container runtime foundation - ([54f42c4](https://github.com/lauritsk/hatchctl/commit/54f42c48623c036951fb4f8bb9d8848da10efbb1)) - Karl Hans Laurits
#### Bug Fixes
- harden archive extraction and CI checks - ([129c6e7](https://github.com/lauritsk/hatchctl/commit/129c6e7b00c29080921b2bd2cdcf019dd35127d4)) - Karl Hans Laurits
- tighten permissions on generated runtime files - ([c44cdc6](https://github.com/lauritsk/hatchctl/commit/c44cdc617eb71217d1fcb8654b7eba95d73196d1)) - Karl Hans Laurits
- preserve compose override mount semantics - ([7af0099](https://github.com/lauritsk/hatchctl/commit/7af00998bc1b5a955a5244592f9bfa3981abf6e1)) - Karl Hans Laurits
- harden bridge state and remote feature resolution - ([e6e0e5d](https://github.com/lauritsk/hatchctl/commit/e6e0e5d86035df7152c5dc51c0e8fc7080a936ad)) - Karl Hans Laurits
- treat feature option env values as data during builds - ([e196153](https://github.com/lauritsk/hatchctl/commit/e196153dbc7baac41e784167c5b1a812b67805cf)) - Karl Hans Laurits
- make compose override generation ephemeral - ([285c73f](https://github.com/lauritsk/hatchctl/commit/285c73f450a9771074924fbc75f810a48405c9bb)) - Karl Hans Laurits
- separate read-only resolution and package bridge helper - ([49d00b1](https://github.com/lauritsk/hatchctl/commit/49d00b1c1c4a746fab3d17fe668bdd2c0ec27f94)) - Karl Hans Laurits
- correct linux bridge integration assertions - ([ec63898](https://github.com/lauritsk/hatchctl/commit/ec63898d3e5c7e4c6f4e927abaf060c2fd1162d2)) - Karl Hans Laurits
#### Documentation
- prepare repo for public v0.1.0 release - ([e153292](https://github.com/lauritsk/hatchctl/commit/e1532923bb4e88322bf893d106789ecba3fd249e)) - Karl Hans Laurits
- update completed todo items - ([b7d0ea8](https://github.com/lauritsk/hatchctl/commit/b7d0ea8e3d37f3236dd42bc327a88a1c34206466)) - Karl Hans Laurits
- clarify compose override lifecycle - ([9d1e21e](https://github.com/lauritsk/hatchctl/commit/9d1e21e86470e37c59ede8ff78300c192929fbad)) - Karl Hans Laurits
- rewrite project README - ([7146436](https://github.com/lauritsk/hatchctl/commit/7146436e322e883e38fced5fd1d994c192f1705c)) - Karl Hans Laurits
#### Tests
- add fixture-backed config regressions - ([c7f24e9](https://github.com/lauritsk/hatchctl/commit/c7f24e9366aa243dbfb54672148b40db80994cf9)) - Karl Hans Laurits
- provide bridge helper in integration tests - ([7471510](https://github.com/lauritsk/hatchctl/commit/74715109d0863960750a5d6049f7298b556b74e6)) - Karl Hans Laurits
- cover read-only runtime and helper lookup - ([619d01a](https://github.com/lauritsk/hatchctl/commit/619d01a69c9e33217a6d6e15d1b9594f6f03243c)) - Karl Hans Laurits
#### Refactoring
- migrate CLI to Cobra and terminal UI events - ([4c1baa4](https://github.com/lauritsk/hatchctl/commit/4c1baa4f3cc0e9fb393587479f1fad315e363621)) - Karl Hans Laurits
- deduplicate small runtime helpers - ([6f499c4](https://github.com/lauritsk/hatchctl/commit/6f499c44a0f45173fff9cd1480b066f7eb64bb21)) - Karl Hans Laurits
- centralize runtime prepare flow - ([cc0f701](https://github.com/lauritsk/hatchctl/commit/cc0f7016cdc5e38e3f1d716fca2b4f0691ca66ab)) - Karl Hans Laurits
- inject runtime stdio and host command execution - ([7265f3e](https://github.com/lauritsk/hatchctl/commit/7265f3eda30e592057470819a62025d332c60d36)) - Karl Hans Laurits
- simplify runtime helpers and docker errors - ([15fba98](https://github.com/lauritsk/hatchctl/commit/15fba98e22bfbbaacbddf355887f13897271a6d0)) - Karl Hans Laurits
- split runtime image and container code - ([c4e3139](https://github.com/lauritsk/hatchctl/commit/c4e31390c949be6051761e2069b5e77d47fd4c3c)) - Karl Hans Laurits
- split runtime compose and lifecycle code - ([de0fc3f](https://github.com/lauritsk/hatchctl/commit/de0fc3fc725b0a4586bf4573e72ebb890eac507b)) - Karl Hans Laurits
#### Miscellaneous Chores
- configure Cocogitto defaults - ([13f095c](https://github.com/lauritsk/hatchctl/commit/13f095c25075c37be116c57c02edb2d881071e86)) - Karl Hans Laurits
- remove leftover compile anchor - ([c98bbf0](https://github.com/lauritsk/hatchctl/commit/c98bbf0989dfa3768254b8365a73ac2251465676)) - Karl Hans Laurits
- initialize hatchctl repository - ([8d08d1b](https://github.com/lauritsk/hatchctl/commit/8d08d1ba8510d10cc98236e0c8fb3b1a684abd22)) - Karl Hans Laurits

- - -

Changelog generated by [cocogitto](https://github.com/cocogitto/cocogitto).
