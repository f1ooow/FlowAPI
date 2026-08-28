# 项目知识库

> 长期有效的参考资料、外部文档、产品决策和踩坑记录。与 frontend/backend spec 不同，这里放的不是"代码怎么写"，而是"项目的事实和背景"。

## 目录结构

按需创建以下子目录：

| 目录/文件 | 放什么 | 示例 |
|-----------|--------|------|
| `project-docs/` | 产品规格、UI 规范、架构说明、实施路线图 | product-spec.md, ui-design-spec.md |
| `reference-screenshots/` | 竞品截图、设计参考、用户手册 PDF | libtv-usage-guide.pdf |
| `api-docs/` | 外部 API 文档、提示词指南 | seedance-api-reference.md |
| `glossary.md` | 项目领域词汇表 | "Shot = 单个镜头节点" |
| `roadmap.md` | 产品阶段路线图（Phase 级别） | Phase A -> B -> C -> D |
| `gotchas.md` | 踩坑记录 | "上传超过 5MB 时 nginx 会 413" |
| `decisions/` | 架构决策记录（ADR） | 0001-choose-sqlite.md |

## 使用规则

- bootstrap 时从现有 docs/ 和 .recallos/ 迁移内容到此
- 日常开发中 trellis-update-spec 发现新知识时写入对应位置
- grill-with-docs Mode B 校准时扫描此目录检查过时/缺失
- 二进制文件（PDF、图片）直接放，不需要转格式
