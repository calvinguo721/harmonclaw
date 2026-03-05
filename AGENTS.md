# HarmonClaw 48小时闪击战 - 核心作战地图

## Phase 1：搭建基础骨架 (当前阶段)
目标：建立 Go 后端基础目录、接口声明和 Makefile。
1. 创建以下目录结构：
   - `cmd/harmonclaw/` (存放 main.go 入口)
   - `gateway/` (HTTP 路由)
   - `llm/` (大模型对接)
   - `viking/` (TXT 记忆文件系统)
   - `sandbox/` (安全拦截模块)
2. 在每个目录下创建基础的 `.go` 文件，**只写 Package 声明、核心 Struct 和 Interface 接口，绝对不要写任何具体的业务逻辑代码。**
3. 在根目录下编写一个 `Makefile`，包含以下命令：
   - `run`: 本地运行命令
   - `build`: 本地编译命令
   - `check-rv2`: 交叉编译测试命令 (CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o NUL ./cmd/harmonclaw/)
   - `clean`: 清理命令