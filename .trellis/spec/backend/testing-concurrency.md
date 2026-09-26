# 并发测试约定

## Gin 全局模式

`gin.SetMode` 修改包级全局变量。在调用 `t.Parallel()` 的测试包中，需要 TestMode 时只在该包的 `TestMain` 中设置一次，且所有测试体与 `t.Cleanup` 均不再改模式。参考 `relay/channel/api_request_test.go`、`relay/channel/gemini/relay_gemini_usage_test.go`。

```go
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
```

## 异步任务与 fixture 收尾

当测试调用的生产路径向 `gopool.Go` 投递任务，而任务读取测试临时设置的全局开关或数据库时，cleanup 必须在任务结束后才恢复开关、替换数据库或关闭连接。`controller/task_submission_test.go` 在 teardown 中等待 `gopool.WorkerCount() == 0`；该测试包的用例不并行运行，因此没有新的同包任务在等待期间入队。不能用固定 `Sleep` 猜测任务何时结束。

## 共享对象断言

轮询协程会通过 GORM 更新传入的 `*model.Task`。测试应在启动协程前保存要比较的 upstream ID 字符串，并在 `Eventually` 中检查线程安全的 adaptor 记录值。不要在断言闭包里读取同一 Task 指针；参考 `service/task_polling_test.go` 的慢渠道测试。

相关验证：`go test -race ./relay/... ./controller ./service -count=1`。
