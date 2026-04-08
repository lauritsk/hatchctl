# Changelog
All notable changes to this project will be documented in this file. See [conventional commits](https://www.conventionalcommits.org/) for commit guidelines.

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