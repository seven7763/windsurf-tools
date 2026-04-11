# Windsurf Tools Linux

一个独立的 Linux 控制台项目，用来兼容当前桌面版使用的 `accounts.json` / `settings.json` 数据目录，并提供本地账号管理与额度视图。

这个子项目故意只做安全边界内的能力：

- 复用现有 `WindsurfTools` 数据目录与 JSON 结构
- 提供账号列表、搜索、标签/备注维护、额度字段维护
- 展示基础统计、套餐分布、即将到期与低额度账号
- 默认只监听 `127.0.0.1`

这个子项目不实现以下能力：

- MITM
- 流量拦截
- 自动账号轮换
- 额度绕过或配额规避

## 运行

```bash
cd windsurf-tools-linux
go run ./cmd/windsurf-tools-linux
```

默认地址：

- `http://127.0.0.1:8090`

可选参数：

- `--addr 127.0.0.1:8090`
- `--data-dir /path/to/WindsurfTools`
- `--read-only`

## 数据目录

默认情况下会沿用桌面版的数据目录约定：

- Linux: `$XDG_CONFIG_HOME/WindsurfTools` 或 `~/.config/WindsurfTools`
- 也兼容历史目录 `windsurf-tools-wails` 的一次性迁移

如果你希望直接读取现有机器上的数据，可以显式指定：

```bash
go run ./cmd/windsurf-tools-linux --data-dir ~/.config/WindsurfTools
```

## 验证

```bash
go test ./...
go build ./cmd/windsurf-tools-linux
```
