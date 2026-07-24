# Harness Assembly Publication V2 实施记录

## 1. 状态

- H4已获用户确认。
- Harness Owner P0 additive合同、publisher、reference memory backend和故障测试已落地。
- 未建立production root；未修改Runtime/Application/Host或六组件。

## 2. 落地清单

- [x] `PublicationID = Derive(InputDigest, GenerationID)`；
- [x] 四个immutable historical objects与共同current；
- [x] staged partial不可由Historical/Current Reader读取；
- [x] expected revision+digest current CAS与单调revision；
- [x] Historical/Current Reader分离；
- [x] same-content staged幂等、drift conflict、ABA阻断；
- [x] stage/commit lost reply exact Inspect；
- [x] 每个stage写点崩溃恢复与commit write-point恢复；
- [x] 64并发单winner；
- [x] TTL、clock rollback、typed-nil、defensive clone；
- [ ] durable State Plane backend及其真实事务/commit-marker conformance；
- [ ] H4 HostV2只读接入与SystemReady闭包。

## 3. 完成门

本轮完成只表示Harness Publication V2合同和reference原子可见性已验证。production使用仍要求durable backend、跨进程并发/重启测试、H4 Binding/Application/Host闭包与独立认证；`MemoryStoreV2`不得注册为production backend。
