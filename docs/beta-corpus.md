# Public beta corpus

The beta corpus records the repository URL, pinned commit, package manager, Node runtime, observed findings, false positives, and unsupported constructs for every external validation sample. Cloning is performed read-only and each sample is run in static mode with a before/after hash check.

## Current local gate

The checked-in regression corpus contains synthetic, redistributable reproductions:

| Sample | Purpose | Expected result |
| --- | --- | --- |
| `testdata/repos/clean-node` | compatible Node/npm/documentation facts | no finding status |
| `testdata/repos/drifted-node` | Node, manager, script, and workflow drift | evidence-backed findings |
| `testdata/repos/malicious-docs` | redaction and timeout behavior | host command failure without leaked token |

## External samples

These read-only snapshots were checked with the static analyzer on 2026-08-13. The snapshots are not redistributed here. The findings column records observed rule output; it is not a maintainer adjudication. Candidate findings require owner review before a public release.

| URL | Commit SHA | Manager | Node evidence | Observed finding rules | Notes |
| --- | --- | --- | --- | --- | --- |
| https://github.com/expressjs/express | `a3714473feb3d2908add734d340e7755fd85e0a3` | npm | `>= 18`, workflow matrix | none | 12 inconclusive workflow/version results |
| https://github.com/fastify/fastify | `e4ffc205328db294d550c5855d2573b33f5e9d62` | npm, pnpm, Yarn | `20`, `24`, `26`, `lts/*` | `package-manager-conflict` | 49 inconclusive results; mixed-manager CI needs maintainer review |
| https://github.com/prettier/prettier | `8f2e4a6682cb2d880d144198fe51f62c558e2c4c` | Yarn | `>=22`, `20`, `22`, `24` | `missing-package-script` | 55 inconclusive results |
| https://github.com/sindresorhus/p-map-series | `bc1b9f5e19ed62363bff3d7dc5ecc1fd820ccb51` | npm | `>=12`, `12`, `14` | `node-version-conflict` | 1 inconclusive result |
| https://github.com/sindresorhus/p-map | `bc26cf03f81292325236a1188063dac8e7a4de0f` | npm | `>=18`, `18`, `20` | `node-version-conflict` | 1 inconclusive result |
| https://github.com/chalk/chalk | `661317e6f91fe7c90306c2c48ea9354562ee9146` | npm | `>=22`, `22`, `26` | `node-version-conflict` | 1 inconclusive result |
| https://github.com/pnpm/pnpm | `e240d17840811b96b21f14cd689837540a953ae5` | pnpm | no literal Node constraint | none | 180 inconclusive workflow results |
| https://github.com/yarnpkg/berry | `57081c05a398f25c92df1dc78752f2053576cec0` | Yarn | `>=18.12.0` | none | 129 inconclusive workflow results |
| https://github.com/npm/cli | `51c2bf81fa2c31547d0fec44fff2aaac3d9a9862` | npm | `^22.22.2 || ^24.15.0 || >=26.0.0`, `26.x` | none | 330 inconclusive workflow results |
| https://github.com/vitejs/vite | `5e7efa0087806738f2737fb2b0982569a8444dee` | pnpm | `^20.19.0 || >=22.12.0`, `24` | none | 24 inconclusive workflow results |

The three direct candidate findings in the small package samples and the Fastify mixed-manager candidate are not yet confirmed true findings or false positives. Every confirmed false positive must become a synthetic regression test before release. At least three external maintainers should interpret a finding without internal guidance.
