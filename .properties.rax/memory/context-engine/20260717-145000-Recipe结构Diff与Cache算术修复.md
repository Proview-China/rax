# Recipe结构Diff与Cache算术修复

时间：2026-07-17 14:50 CST

Context Owner新增`ContextRecipeComparisonV1`纯结构报告与`kernel.CompareContextRecipesV1`。报告exact绑定Base/Candidate Recipe，覆盖ID、SemanticVersion、Revision、Owner、Rules内容/增删/顺序、Budget、RenderVersion及lifetime，变化按field path规范排序且只携带before/after digest。它不判断better、compatible或publish，不写lifecycle/current，也不替代Evaluation/Review。

Cache economics修复了两个大数问题：乘法溢出后再除会产生错误成本，以及`write + keepalive`可能发生`uint64`回绕。当前实现用精确任意精度中间乘除，只在最终值超出`uint64`时饱和，并对加法单独饱和。反例覆盖“乘积溢出但除后仍可表示”、read savings最终饱和及write+keepalive回绕。

定向验证：`go test -count=100 ./contract ./kernel`与`go test -race -count=20 ./contract ./kernel`均PASS。该工作无外部Effect、Runtime Settlement、Provider调用或跨Owner依赖。
