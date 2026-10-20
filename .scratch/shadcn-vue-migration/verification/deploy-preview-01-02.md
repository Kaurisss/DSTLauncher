# 01/02 票容器预览部署

## 部署结果

- 用户授权后重建 `dst-panel:local` 并重建 `dst-panel` 容器。
- 容器为 running / healthy，面板地址 `http://127.0.0.1:16800/`，仍仅发布到宿主回环。
- 镜像：`sha256:a662c620b78e6ebe41421896f54d6d58a5ab16f3d440b96e24005890292e2244`。
- 原 `./data:/data` 挂载保持不变。部署前旧容器已经退出；部署过程未删除数据或触发游戏安装、开服。
- 回滚镜像标签：`dst-panel:before-shadcn-20261020-154004`。
- 最终资源：`index-CflR4OKr.js`、`index-v2SfnMv_.css`；生产构建包含本地字体与许可证。

## 验证

- 健康检查 HTTP 200；安装、房间、服务器配置 API 均返回 code 200。
- 实际后端保留 2 个房间，0 个活动分片；服务器文件尚未安装完成，未运行下载任务。
- 未注入 fixture 的真实浏览器页面显示七个导航入口、实际房间、已停止状态；本地字体加载正常，控制台无错误/警告。
- 修复真实后端成功返回空分片数组时仍显示等待的问题：单独记录状态是否已加载，换房重置；新增回归先失败后通过。
- 前端最终回归 25/25 通过，lint 零告警；Docker 内前端与 Linux Go 构建通过。
- 重建前再次检查安装、模组下载和开服任务，仅在后端空闲时替换容器。

## 依赖审计记录

首次 Docker npm ci 报告 11 项告警。使用官方 registry 的 `npm audit --registry=https://registry.npmjs.org --json` 核实为 5 moderate / 6 high，均归属于构建用 shadcn-vue CLI 的依赖链，涉及 detypes、vue-metamorph、fast-glob、micromatch、braces、stylus/source-map-resolve 等。未执行自动修复或 CLI 降级；留给最终依赖收口评估。运行镜像按多阶段 Dockerfile 只复制 Go 二进制及其嵌入的静态资源。

本次交付 01/02 的预览部署；03～08 按原工单计划继续推进。
