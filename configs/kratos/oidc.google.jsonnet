// Google 社交登录映射配置
// 将 Google 的用户信息映射到 Kratos 的 identity traits
// 文档: https://www.ory.sh/docs/kratos/social-signin/data-mapping

function(ctx) {
  // ctx.vault.access_token 包含 Google OAuth2 的访问令牌
  // ctx.vault.id_token 包含 Google OpenID Connect 的 ID 令牌
  // ctx.raw 包含原始的用户信息响应

  // 使用 Google 提供的 email 作为主要标识
  email: ctx.raw.email,
  username: ctx.raw.email,
}
