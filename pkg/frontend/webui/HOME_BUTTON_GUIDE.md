# 返回主页按钮使用指南

## 概述

所有页面都应该提供返回主页的按钮，方便用户快速导航回首页。本文档提供了通用的样式和实现方案。

## 通用样式（推荐）

在页面的 `<style>` 部分添加以下 CSS：

```css
.back-link {
    display: inline-block;
    color: white;  /* 或其他适合背景的颜色 */
    text-decoration: none;
    margin-bottom: 8px;  /* 根据实际布局调整 */
    opacity: 0.9;
    transition: opacity 0.2s;
    font-size: 14px;
}
.back-link:hover {
    opacity: 1;
}
```

## 实现示例

### 方式一：白色背景页面（如 API 文档）

**CSS 样式：**
```css
.header {
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
    color: white;
    padding: 24px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}
.back-link {
    display: inline-block;
    color: white;
    text-decoration: none;
    margin-bottom: 16px;
    opacity: 0.9;
    transition: opacity 0.2s;
}
.back-link:hover {
    opacity: 1;
}
```

**HTML 结构：**
```html
<div class="header">
    <a href="%s/" class="back-link">← 返回首页</a>
    <h1>📚 API Documentation</h1>
    <div class="subtitle">YuanRong Serverless Platform REST API 参考文档</div>
</div>
```

**Go 代码：**
```go
func HandleYourPage(w http.ResponseWriter, r *http.Request) {
    pathPrefix := r.Header.Get("X-Forwarded-Prefix")
    
    html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <!-- ... -->
</head>
<body>
    <div class="header">
        <a href="%s/" class="back-link">← 返回首页</a>
        <h1>Your Page Title</h1>
    </div>
    <!-- ... -->
</body>
</html>`, pathPrefix)
    
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write([]byte(html))
}
```

### 方式二：深色背景页面（如 Terminal）

**CSS 样式：**
```css
#header {
    background: #2d2d30;
    color: #ccc;
    padding: 10px 20px;
    border-bottom: 1px solid #3e3e42;
    display: flex;
    justify-content: space-between;
    align-items: center;
}
#header .left-section {
    display: flex;
    align-items: center;
    gap: 16px;
}
.back-link {
    color: #ccc;
    text-decoration: none;
    opacity: 0.8;
    transition: opacity 0.2s;
    font-size: 14px;
}
.back-link:hover {
    opacity: 1;
}
```

**HTML 结构：**
```html
<div id="header">
    <div class="left-section">
        <a href="%s/" class="back-link">← 首页</a>
        <h1>🖥️ Remote Exec Terminal</h1>
    </div>
    <div id="status">
        <!-- 其他状态信息 -->
    </div>
</div>
```

## 注意事项

### 1. pathPrefix 参数

确保在 `fmt.Sprintf` 调用中为返回链接提供 `pathPrefix` 参数：

```go
// 统计模板中 %s 的数量
// 确保 pathPrefix 参数数量与模板中的 %s 数量匹配

html := fmt.Sprintf(`...
    <a href="%s/" class="back-link">← 返回首页</a>
    ...
`, pathPrefix, pathPrefix, ...) // 根据实际 %s 数量添加
```

### 2. 链接格式

- **正确**：`href="%s/"` - 链接到首页
- **错误**：`href="/"` - 可能导致在有路径前缀时无法正确跳转

### 3. 文本内容

根据页面类型选择合适的文本：
- 简短页面：`← 首页`
- 详细页面：`← 返回首页`
- 英文页面：`← Home` 或 `← Back to Home`

### 4. 位置建议

- **标准页面**：放在 header 顶部，标题上方
- **固定 header**：放在 header 左侧，与标题同行
- **全屏应用**：放在左上角，使用图标 + 文字

## 快速检查清单

创建新页面时，请检查：

- [ ] 已添加 `.back-link` CSS 样式
- [ ] 已在 HTML 中添加返回链接
- [ ] 返回链接使用了 `%s/` 作为 href
- [ ] `fmt.Sprintf` 的 pathPrefix 参数数量正确
- [ ] 链接在有无路径前缀时都能正常工作
- [ ] 鼠标悬停时有视觉反馈（opacity 变化）

## 测试方法

### 本地测试
```bash
# 直接访问（无路径前缀）
curl http://localhost:8080/your-page

# 带路径前缀测试
curl -H "X-Forwarded-Prefix: /frontend" http://localhost:8080/your-page
```

### 浏览器测试
1. 访问页面
2. 点击返回主页按钮
3. 确认能正确跳转到首页
4. 如果使用 Traefik 等代理，测试带路径前缀的场景

## 现有页面参考

可以参考以下已实现的页面：

- **白色背景 + 渐变 header**：[apidoc.go](apidoc.go)
- **白色背景 + 表单布局**：[invoke.go](invoke.go)
- **深色背景 + 终端风格**：[webterm.go](webterm.go)

---

*最后更新：2026-02-14*
