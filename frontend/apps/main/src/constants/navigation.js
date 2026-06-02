import {
  AddressCard,
  Bolt,
  ChartBar,
  CircleDot,
  CircleUser,
  Clock,
  DiagramNested,
  Envelope,
  Eye,
  FileLines,
  Gear,
  House,
  Inbox,
  Key,
  MessageBot,
  Plug,
  Scroll,
  Shield,
  Sliders,
  Tags,
  Timer,
  User,
  UserMagnifyingGlass,
  Users,
  Vault,
  Webhook,
  Wrench
} from '@qall/icons'

export const reportsNavItems = [
  {
    titleKey: 'globals.terms.overview',
    href: '/reports/overview',
    permission: 'reports:manage',
    icon: ChartBar
  }
]

export const adminHomeNavItem = {
  titleKey: 'admin.nav.home.title',
  href: '/admin',
  icon: House
}

export const adminNavItems = [
  {
    titleKey: 'globals.terms.workspace',
    iconBgClass: 'bg-purple-200 dark:bg-purple-950/35',
    children: [
      {
        titleKey: 'globals.terms.general',
        descriptionKey: 'admin.nav.general.description',
        href: '/admin/general',
        permission: 'general_settings:manage',
        icon: Gear
      },
      {
        titleKey: 'globals.terms.businessHour',
        descriptionKey: 'admin.nav.businessHours.description',
        href: '/admin/business-hours',
        permission: 'business_hours:manage',
        isTitleKeyPlural: true,
        icon: Clock
      },
      {
        titleKey: 'globals.terms.slaPolicy',
        descriptionKey: 'admin.nav.slaPolicies.description',
        href: '/admin/sla',
        permission: 'sla:manage',
        isTitleKeyPlural: true,
        icon: Timer
      }
    ]
  },
  {
    titleKey: 'globals.terms.channel',
    isTitleKeyPlural: true,
    iconBgClass: 'bg-pink-200 dark:bg-pink-950/35',
    children: [
      {
        titleKey: 'globals.terms.inbox',
        descriptionKey: 'admin.nav.inboxes.description',
        href: '/admin/inboxes',
        permission: 'inboxes:manage',
        isTitleKeyPlural: true,
        icon: Inbox
      }
    ]
  },
  {
    titleKey: 'globals.terms.conversation',
    isTitleKeyPlural: true,
    iconBgClass: 'bg-blue-200 dark:bg-blue-950/35',
    children: [
      {
        titleKey: 'globals.terms.status',
        descriptionKey: 'admin.nav.statuses.description',
        href: '/admin/conversations/statuses',
        permission: 'status:manage',
        isTitleKeyPlural: true,
        icon: CircleDot
      },
      {
        titleKey: 'globals.terms.tag',
        descriptionKey: 'admin.nav.tags.description',
        href: '/admin/conversations/tags',
        permission: 'tags:manage',
        isTitleKeyPlural: true,
        icon: Tags
      },
      {
        titleKey: 'globals.terms.customAttribute',
        descriptionKey: 'admin.nav.customAttributes.description',
        href: '/admin/custom-attributes',
        permission: 'custom_attributes:manage',
        isTitleKeyPlural: true,
        icon: Sliders
      },
      {
        titleKey: 'globals.terms.sharedView',
        descriptionKey: 'admin.nav.sharedViews.description',
        href: '/admin/conversations/shared-views',
        permission: 'shared_views:manage',
        isTitleKeyPlural: true,
        icon: Eye
      }
    ]
  },
  {
    titleKey: 'globals.terms.productivity',
    iconBgClass: 'bg-amber-200 dark:bg-amber-950/35',
    children: [
      {
        titleKey: 'globals.terms.macro',
        descriptionKey: 'admin.nav.macros.description',
        href: '/admin/conversations/macros',
        permission: 'macros:manage',
        isTitleKeyPlural: true,
        icon: Bolt
      },
      {
        titleKey: 'globals.terms.automation',
        descriptionKey: 'admin.nav.automations.description',
        href: '/admin/automations',
        permission: 'automations:manage',
        isTitleKeyPlural: true,
        icon: DiagramNested
      }
    ]
  },
  {
    titleKey: 'globals.terms.teammate',
    isTitleKeyPlural: true,
    iconBgClass: 'bg-emerald-200 dark:bg-emerald-950/35',
    children: [
      {
        titleKey: 'globals.terms.agent',
        descriptionKey: 'admin.nav.agents.description',
        href: '/admin/teams/agents',
        permission: 'users:manage',
        isTitleKeyPlural: true,
        icon: User
      },
      {
        titleKey: 'globals.terms.team',
        descriptionKey: 'admin.nav.teams.description',
        href: '/admin/teams/teams',
        permission: 'teams:manage',
        isTitleKeyPlural: true,
        icon: Users
      },
      {
        titleKey: 'globals.terms.role',
        descriptionKey: 'admin.nav.roles.description',
        href: '/admin/teams/roles',
        permission: 'roles:manage',
        isTitleKeyPlural: true,
        icon: Shield
      },
      {
        titleKey: 'globals.terms.activityLog',
        descriptionKey: 'admin.nav.activityLog.description',
        href: '/admin/teams/activity-log',
        permission: 'activity_logs:manage',
        isTitleKeyPlural: true,
        icon: Scroll
      }
    ]
  },
  {
    titleKey: 'globals.terms.notification',
    isTitleKeyPlural: true,
    iconBgClass: 'bg-sky-200 dark:bg-sky-950/35',
    children: [
      {
        titleKey: 'globals.terms.email',
        descriptionKey: 'admin.nav.emailNotifications.description',
        href: '/admin/notification',
        permission: 'notification_settings:manage',
        icon: Envelope
      },
      {
        titleKey: 'globals.terms.template',
        descriptionKey: 'admin.nav.templates.description',
        href: '/admin/templates',
        permission: 'templates:manage',
        isTitleKeyPlural: true,
        icon: FileLines
      }
    ]
  },
  {
    titleKey: 'globals.terms.security',
    iconBgClass: 'bg-rose-200 dark:bg-rose-950/35',
    children: [
      {
        titleKey: 'globals.terms.sso',
        descriptionKey: 'admin.nav.sso.description',
        href: '/admin/sso',
        permission: 'oidc:manage',
        icon: Key
      },
      {
        titleKey: 'globals.terms.secret',
        descriptionKey: 'admin.nav.secrets.description',
        href: '/admin/security/secrets',
        permission: 'secrets:manage',
        isTitleKeyPlural: true,
        icon: Vault
      }
    ]
  },
  {
    titleKey: 'globals.terms.integration',
    isTitleKeyPlural: true,
    iconBgClass: 'bg-violet-200 dark:bg-violet-950/35',
    children: [
      {
        titleKey: 'globals.terms.webhook',
        descriptionKey: 'admin.nav.webhooks.description',
        href: '/admin/webhooks',
        permission: 'webhooks:manage',
        isTitleKeyPlural: true,
        icon: Webhook
      }
    ]
  },
  {
    titleKey: 'admin.nav.erpIntegration',
    icon: UserMagnifyingGlass,
    iconBgClass: 'bg-orange-200 dark:bg-orange-950/35',
    children: [
      {
        titleKey: 'admin.customerIdentification.title',
        descriptionKey: 'admin.nav.customerIdentification.description',
        href: '/admin/erp/customer-identification',
        permission: 'customers:manage',
        icon: UserMagnifyingGlass
      },
      {
        titleKey: 'admin.customerIdentification.erpConnectorsTitle',
        descriptionKey: 'admin.nav.erpConnectors.description',
        href: '/admin/erp/connectors',
        permission: 'customers:manage',
        icon: Plug
      }
    ]
  }
]

// AI is its own top-level area (sibling to inbox/knowledge/admin in the icon
// rail), not nested under /admin. v1 is a flat list of two items; the future
// plan groups them as Configure / Operate / Quality / Analytics once a third
// item lands. See F9 navigation proposal.
export const aiNavItems = [
  {
    titleKey: 'globals.terms.aiAgent',
    href: '/ai/agents',
    permission: 'ai_agents:manage',
    isTitleKeyPlural: true,
    icon: MessageBot
  },
  {
    titleKey: 'globals.terms.aiTool',
    href: '/ai/tools',
    permission: 'ai_tools:manage',
    isTitleKeyPlural: true,
    icon: Wrench
  }
]

export const accountNavItems = [
  {
    titleKey: 'globals.terms.profile',
    href: '/account/profile',
    icon: CircleUser
  }
]

export const contactNavItems = [
  {
    titleKey: 'globals.terms.contact',
    allLabelKey: 'contact.allContacts',
    href: '/contacts',
    isTitleKeyPlural: true,
    icon: AddressCard
  }
]
