# Vitest 浏览器存储环境

`web/vitest.config.ts` 使用 jsdom。Node 25 预置的 `globalThis.localStorage` 在未配置 `--localstorage-file` 时缺少 `setItem`/`clear`，会覆盖 Vitest 对 jsdom Storage 的全局绑定，导致 Zustand persist 和组件测试失败。

`web/src/test-setup.ts` 在每个 jsdom 测试环境初始化时，把 `globalThis.localStorage` 绑定到 `jsdom.window.localStorage`。新增需要浏览器存储的测试直接使用标准 Storage API，不依赖开发者设置 Node 启动参数，也不要用生产组件的 mock 掩盖存储问题。

验证命令：在 Node 25 下从 `web/` 直接执行 `bun run test`，无需 `NODE_OPTIONS`。
