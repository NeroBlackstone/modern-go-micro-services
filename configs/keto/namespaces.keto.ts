// Copyright © 2026 Ory Corp
// SPDX-License-Identifier: Apache-2.0

// Keto Namespace 定义 - 使用 OPL (Ory Permission Language)
// 文档: https://www.ory.sh/keto/docs/guides/define-permissions

import { Namespace, SubjectSet, Context } from "@ory/keto-namespace-types"

// User 命名空间 - 用户身份
class User implements Namespace {}

// Group 命名空间 - 用户组（用于通配符权限）
class Group implements Namespace {
  related: {
    // 组包含的成员
    members: User[]
  }
}

// Role 命名空间 - 角色定义
class Role implements Namespace {
  related: {
    // 角色包含的成员
    members: User[]
  }
}

// Book 命名空间 - 书籍资源
class Book implements Namespace {
  related: {
    // 可管理书籍的角色
    managers: SubjectSet<Role, "members">[]
    // 可编辑书籍的角色
    editors: SubjectSet<Role, "members">[]
    // 可查看书籍的用户或用户组
    viewers: (User | SubjectSet<Role, "members"> | SubjectSet<Group, "members">)[]
  }

  permits = {
    // 管理权限: 管理员可管理所有书籍
    manage: (ctx: Context): boolean =>
      this.related.managers.includes(ctx.subject),
    // 编辑权限: 管理员或编辑者可编辑书籍
    edit: (ctx: Context): boolean =>
      this.related.managers.includes(ctx.subject) ||
      this.related.editors.includes(ctx.subject),
    // 查看权限: 所有用户可查看书籍 - 通过 viewers 关系
    read: (ctx: Context): boolean =>
      this.related.viewers.includes(ctx.subject),
  }
}

// Order 命名空间 - 订单资源
class Order implements Namespace {
  related: {
    // 可管理订单的角色
    managers: SubjectSet<Role, "members">[]
    // 可查看订单的用户或用户组
    viewers: (User | SubjectSet<Role, "members"> | SubjectSet<Group, "members">)[]
  }

  permits = {
    // 管理权限: 管理员可管理所有订单
    manage: (ctx: Context): boolean =>
      this.related.managers.includes(ctx.subject),
    // 查看权限: 所有登录用户可查看订单
    read: (ctx: Context): boolean =>
      this.related.viewers.includes(ctx.subject),
  }
}

// Review 命名空间 - 书评资源
class Review implements Namespace {
  related: {
    // 可管理书评的角色
    managers: SubjectSet<Role, "members">[]
    // 可发布书评的用户或用户组
    writers: (User | SubjectSet<Role, "members"> | SubjectSet<Group, "members">)[]
  }

  permits = {
    // 管理权限: 管理员可管理所有书评
    manage: (ctx: Context): boolean =>
      this.related.managers.includes(ctx.subject),
    // 发布权限: 登录用户可发布书评
    write: (ctx: Context): boolean =>
      this.related.writers.includes(ctx.subject),
  }
}

