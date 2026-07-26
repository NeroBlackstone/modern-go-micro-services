// GitHub 社交登录映射配置
// 将 GitHub 的用户信息映射到 Kratos 的 identity traits
// 文档: https://www.ory.sh/docs/kratos/social-signin/data-mapping

function(ctx) {
  // ctx.raw 包含 GitHub API 返回的用户信息
  // GitHub 用户信息端点: https://api.github.com/user

  // 使用 GitHub 的 primary email 作为主要标识
  email: ctx.raw.email,
  // 使用 GitHub 的 login 作为用户名
  username: ctx.raw.login,
}
