---
name: sync-upstream-next
description: 将最新 origin/main 线性同步到 next，逐项核对上游新增提交是否完整覆盖 next 对应实现的全部必要行为；仅在完整覆盖时舍弃本地实现，部分覆盖时以上游为基座保留上游未覆盖的必要差异。用于更新 main、同步上游到 next、rebase/线性化 next、消除 merge 节点，或处理上游后来实现了本地功能的场景。
---

## 目标

以最新 `origin/main` 为不可修改的基座，重建线性的 `next` 历史。上游已经实现的功能不在 `next` 重复维护，只保留必要的本地差异。

## 不变量

- 将 `origin/main` 及其已有提交视为不可修改基座，不修改、amend、重写、压缩、重新生成或 cherry-pick 上游提交
- 本地 `main` 只能 fast-forward 到 `origin/main`
- 不使用 merge 将 `main` 同步到 `next`
- 只允许重写 `next` 独有提交；冲突解决、接口适配和验证修复必须落在对应本地提交或新增的 `next` 本地提交
- 不得因格式、lint、版权、注释或顺手优化，在 `next` 新增提交修改仅由上游引入或变更的代码
- 审计对象是上游新增提交带来的具体功能、运行行为和验证覆盖，不只是提交语义或相同文件
- 只有上游实现完整覆盖本地对应功能及全部必要附加行为时，才删除本地对应实现
- 上游只覆盖核心功能时，以上游为基座，只补上游未覆盖且仍必要的本地差异
- 安全限制、资源边界、兼容或降级处理、接口与数据语义、有效回归测试都必须独立核对，不得当作实现细节静默删除
- 本地提交同时包含独立功能或必要附加行为时，拆分后保留未被上游覆盖的部分
- 重写前保留本地备份引用
- 推送重写历史时必须使用绑定旧远端哈希的显式 `--force-with-lease`

## 工作流

### 1. 先确认方案

在修改分支前说明以下内容并等待用户确认：

- 将 `main` 以 `--ff-only` 更新到 `origin/main`
- 将审计上游新增提交是否完整覆盖 `next` 对应实现的全部必要行为
- 将只重写 `next` 独有提交并保持无 merge 节点
- 将在验证后使用 `--force-with-lease` 更新远程 `next`

若用户未授权历史重写或远程推送，不执行相应步骤。

### 2. 获取基线

先读取仓库规则，再执行只读检查：

```bash
git status --porcelain=v1 --branch
git branch -vv
git fetch origin --prune
git rev-parse main origin/main next origin/next
git log --left-right --oneline origin/next...next
git merge-base origin/main origin/next
git log --oneline --reverse origin/next..origin/main
git log --oneline --reverse origin/main..origin/next
git log --left-right --cherry-mark --no-merges --oneline origin/main...origin/next
```

要求：

- 工作区必须干净；不得暂存或提交无关改动
- 记录 fetch 后的 `origin/next` 完整哈希，后续作为 lease，不得在推送前悄悄刷新该值
- 本地 `next` 必须与 `origin/next` 指向同一提交；若存在未推送或分叉提交，停止并让用户确认如何处理，不得重置、遗漏或擅自纳入
- 记录共同基线完整哈希

### 3. 审计上游功能覆盖

以上游新增提交为入口，先列出它新增的具体功能、行为与测试，再列出 `next` 对应实现包含的全部行为。判断的是上游是否完整覆盖本地必要行为，而不是本地是否已经实现上游提交的主题。不能只比较提交标题、改动文件或抽象目的。

核心功能相同但本地还包含安全限制、资源上限、兼容处理或有效测试时，属于部分覆盖，不得整体舍弃本地提交。

逐个查看上游新增提交和 `next` 独有提交：

```bash
git show --stat --summary <commit>
git show <commit> -- <relevant-paths>
```

编辑验证或冲突涉及的文件前，先确认是否有 `next` 独有提交改过该路径：

```bash
git log --oneline origin/main..origin/next -- <path>
```

- 没有本地提交改过该路径时，将其记录为仅上游路径，不做格式、lint、版权、注释或一般优化
- 上游接口变化使本地行为失效时，修改本地调用方或对应本地提交，并补充验证
- 上游代码自身的问题只记录到交付报告，不在同步中修复

按运行行为、接口契约、数据语义和测试意图分类：

| 上游覆盖情况 | 处理 |
|---|---|
| 本地没有对应功能 | 直接采用上游，原样重放无关本地提交 |
| 完整覆盖本地对应功能及必要行为 | 舍弃本地对应实现，完全采用上游提交 |
| 只覆盖核心功能或部分行为 | 采用上游实现，比较代码行为后只补仍缺少的必要差异 |
| 本地提交混有独立功能或必要附加行为 | 拆分提交，舍弃已被上游覆盖的部分并保留其余部分 |
| 实现冲突 | 以上游为基座，只补能够明确证明仍有业务价值的差异 |

发现本地对应实现时，先向用户报告：

- 上游提交与对应本地提交
- 本地对应实现包含的具体功能与必要附加行为
- 上游对每项本地行为的覆盖程度
- 代码行为和验证覆盖的差异
- 建议舍弃、拆分或保留的内容
- 预计验证范围

在用户确认处理方案前不改写历史。没有本地对应实现时，明确报告后可按已确认的总体方案继续。

### 4. 建立备份并更新 main

使用包含旧 `next` 短哈希的唯一名称创建仅本地备份分支：

```bash
git branch backup/next-before-main-sync-<old-next-short-sha> <old-next-full-sha>
git merge-base --is-ancestor main origin/main
git switch main
git merge --ff-only origin/main
```

若本地 `main` 不能 fast-forward，停止并报告，不重写 `main`。

### 5. 线性重建 next

没有需要舍弃或拆分的提交时直接重放：

```bash
git switch next
git rebase main
```

存在需要舍弃或拆分的提交时：

1. 从最新 `main` 创建临时整理分支
2. 按原顺序 cherry-pick 无需拆分的 `next` 独有提交
3. 跳过被上游完整覆盖的本地提交
4. 对部分覆盖的提交使用 `git cherry-pick -n`，精确删除上游已覆盖的补丁，只提交未覆盖的必要差异
5. 验证临时分支后再将本地 `next` 指向整理结果

不要 cherry-pick 上游提交；临时分支从 `main` 开始即可保留上游提交的原始身份。发生冲突时逐项比较语义，禁止无判断地整文件选择 `ours` 或 `theirs`。

### 6. 验证

至少完成：

```bash
git merge-base --is-ancestor main next
git log --merges --oneline main..next
git range-diff <old-base>..<backup-ref> main..next
git diff --check
git status --porcelain=v1 --branch
make test
```

对第 3 步记录的每个仅上游路径执行以下检查；没有此类路径时跳过：

```bash
git diff --exit-code main next -- <upstream-only-path>
```

成功标准：

- `main` 与 `origin/main` 指向同一提交
- `main` 是 `next` 的祖先
- `main..next` 没有 merge commit
- `range-diff` 中未删除、拆分或修改的本地补丁保持一致
- 被舍弃或拆分的补丁与已确认的覆盖报告一致
- 未确认需要本地差异的上游文件与 `main` 内容一致
- 工作区干净，测试通过

若变更触及 `relaykit/` 或其公共 API，必须额外验证：

```bash
cd relaykit
GOWORK=off go build ./...
```

若保留的差异涉及前端，按 `web/AGENTS.md` 运行受影响测试、`bun run typecheck` 和涉及文件的 lint。

验证失败时先按第 3 步确认失败文件的归属：

- 失败来自本地差异时，修复对应本地提交或新增明确的本地兼容提交
- 只有同一命令在干净的 `main` 基线上以相同原因失败，才能报告为上游基线问题；未复现时按本地集成失败处理
- 上游基线问题若阻断构建或测试，不得宣称验证通过
- 不运行会批量改写上游文件的自动修复命令

### 7. 安全推送

使用第 2 步记录的旧远端 `next` 完整哈希：

```bash
git push --force-with-lease=refs/heads/next:<old-origin-next-full-sha> origin next
```

lease 失败表示远端期间发生变化。停止并重新获取、审计，不得改用裸 `--force`。

### 8. 交付报告

报告：

- `main` 与 `next` 的最终完整或短哈希
- 上游新增提交数量和 `next` 独有提交数量
- 每个上游与本地对应项的保留、舍弃或拆分决定
- 为适配上游而调整的本地提交，以及仅存在于上游且未修改的问题
- `range-diff`、无 merge 节点和祖先关系结果
- 实际执行的测试及未执行项
- 本地备份引用名称
- `force-with-lease` 推送结果
