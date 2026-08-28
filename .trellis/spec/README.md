# Zhuyu Universal Fullstack Template

个人定制的 Trellis spec 模板，适用于所有项目。

## 使用方式

```bash
trellis init -u zhuyu --registry https://github.com/你的用户名/trellis-template
```

## 包含内容

- **guides/**: 预填的通用思维指南（实现前检查、代码复用、跨层思考、bug 根因分析）
- **shared/**: 预填的通用规范（代码质量、TypeScript、Git 约定）+ 空的领域词汇表
- **frontend/**: 参考模板（组件、Hooks、状态管理、CSS、React 陷阱）
- **backend/**: 参考模板（API 模式、错误处理、代码质量）
- **big-question/**: 空目录，随开发积累踩坑记录

## 特性

- 中文索引和引导
- 技术栈无关（React/Vue/Svelte + Express/FastAPI/任意后端）
- 领域词汇表（glossary.md）标准化
- bootstrap 任务自动从代码提取规范
