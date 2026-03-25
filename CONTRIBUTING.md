# 贡献指南

感谢您对 Go 并发文件下载工具的兴趣！我们欢迎各种形式的贡献，包括但不限于 bug 报告、功能建议、代码改进和文档完善。

## 开发流程

### 1. 环境设置

```bash
# 克隆仓库
git clone https://github.com/Super-Gagaga/go-Concurrent-download.git
cd go-concurrent-download

# 安装依赖
go mod download

# 运行测试
go test ./...
```

### 2. 代码规范

- 遵循 Go 官方代码规范
- 使用 `gofmt` 格式化代码
- 使用 `go vet` 检查代码问题
- 确保所有导出函数都有文档注释

### 3. 提交信息规范

使用约定式提交格式：

```
<类型>[可选范围]: <描述>

[可选正文]

[可选脚注]
```

类型包括：
- `feat`: 新功能
- `fix`: 修复 bug
- `docs`: 文档更新
- `style`: 代码格式调整
- `refactor`: 代码重构
- `test`: 测试相关
- `chore`: 构建过程或辅助工具的变动

示例：
```
feat: 添加 HTTP/2 协议支持

- 支持 HTTP/2 多路复用
- 优化连接池管理
- 添加相关测试用例
```

### 4. 测试要求

- 所有新功能必须包含相应的测试
- 测试覆盖率保持在 85% 以上
- 运行 `go test -race ./...` 确保无数据竞争

```bash
# 运行测试并生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# 运行竞态检测
go test -race ./...
```

## 功能开发指南

### 1. 添加新的配置选项

1. 在 `pkg/gocd/types.go` 的 `DownloadConfig` 中添加字段
2. 在 `pkg/gocd/downloader.go` 中添加对应的选项函数
3. 更新 `DefaultConfig()` 函数设置默认值
4. 在内部实现中使用新配置
5. 添加相应的测试

### 2. 扩展下载协议

1. 在 `internal/httpclient` 中添加新的协议实现
2. 实现相应的客户端接口
3. 更新连接池以支持新协议
4. 添加协议检测逻辑

### 3. 优化性能

1. 使用性能分析工具定位瓶颈
2. 实现优化后运行基准测试
3. 确保优化不影响功能正确性

```bash
# 性能分析
go test -bench=. -benchmem -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

## 文档维护

### 1. API 文档

- 更新 `docs/API.md` 反映 API 变更
- 确保所有导出函数都有清晰的文档
- 提供使用示例

### 2. 示例代码

- 在 `examples/` 目录下添加新示例
- 示例应该简单明了，展示核心功能
- 包含必要的注释和错误处理

### 3. README 更新

- 更新特性列表
- 添加新的使用示例
- 更新性能数据

## 问题报告

### 1. Bug 报告

请提供以下信息：
- Go 版本
- 操作系统和架构
- 复现步骤
- 期望行为
- 实际行为
- 错误日志（如果有）

### 2. 功能请求

请描述：
- 需要解决的问题
- 建议的解决方案
- 相关的使用场景
- 可能的实现思路

## 代码审查流程

1. 提交 Pull Request
2. 等待 CI 测试通过
3. 维护者审查代码
4. 根据反馈进行修改
5. 通过审查后合并

## CI/CD 流程

项目使用 GitHub Actions 进行自动化测试：
- 单元测试
- 竞态检测
- 代码覆盖率
- 构建验证

## 发布流程

1. 更新版本号
2. 更新 CHANGELOG.md
3. 创建发布标签
4. 构建发布二进制文件
5. 发布到 GitHub Releases

## 联系方式

- GitHub Issues: 问题报告和功能请求
- GitHub Discussions: 技术讨论和问答
- Pull Requests: 代码贡献

感谢您的贡献！