import { cva, type VariantProps } from 'class-variance-authority'

export { default as SidebarContent } from './SidebarContent.vue'
export { default as SidebarGroup } from './SidebarGroup.vue'
export { default as SidebarHeader } from './SidebarHeader.vue'
export { default as SidebarMenu } from './SidebarMenu.vue'
export { default as SidebarMenuAction } from './SidebarMenuAction.vue'
export { default as SidebarMenuButton } from './SidebarMenuButton.vue'
export { default as SidebarMenuItem } from './SidebarMenuItem.vue'
export { default as SidebarMenuSub } from './SidebarMenuSub.vue'
export { default as SidebarMenuSubItem } from './SidebarMenuSubItem.vue'

const sidebarMenuButtonBase =
  'peer/menu-button flex items-center gap-2 overflow-hidden rounded-md p-2 text-left text-sm outline-none ring-sidebar-ring transition-[width,height,padding] hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50 group-has-[[data-sidebar=menu-action]]/menu-item:pr-8 aria-disabled:pointer-events-none aria-disabled:opacity-50 data-[active=true]:bg-sidebar-accent data-[active=true]:font-medium data-[active=true]:text-sidebar-accent-foreground data-[state=open]:hover:bg-sidebar-accent data-[state=open]:hover:text-sidebar-accent-foreground group-data-[collapsible=icon]:!size-8 group-data-[collapsible=icon]:!p-2 [&>span:last-child]:truncate [&>svg]:size-4 [&>svg]:shrink-0'

const sidebarMenuButtonDefault =
  'data-[active=true]:shadow-sm w-full h-10 px-3 text-[13px] font-normal text-foreground mx-2 rounded-md hover:text-primary hover:bg-background dark:hover:bg-zinc-900 data-[active=true]:bg-white data-[active=true]:rounded-lg data-[active=true]:border data-[active=true]:border-muted-background dark:data-[active=true]:bg-zinc-900 data-[active=true]:text-primary [&[data-active=true]_svg]:opacity-100'

const sidebarMenuButtonChild =
  'data-[active=true]:shadow-sm w-full py-3.5 h-10 px-3 text-[13px] font-normal text-foreground mx-2 rounded-md hover:text-primary hover:bg-background dark:hover:bg-zinc-900 data-[active=true]:bg-white data-[active=true]:rounded-lg data-[active=true]:border data-[active=true]:border-muted-background dark:data-[active=true]:bg-zinc-900 data-[active=true]:text-primary [&[data-active=true]_svg]:opacity-100'

export const sidebarMenuButtonVariants = cva(
  sidebarMenuButtonBase,
  {
    variants: {
      variant: {
        default: sidebarMenuButtonDefault,
        child: sidebarMenuButtonChild,
        outline:
          'bg-background shadow-[0_0_0_1px_hsl(var(--sidebar-border))] hover:bg-sidebar-accent hover:text-sidebar-accent-foreground hover:shadow-[0_0_0_1px_hsl(var(--sidebar-accent))]',
      },
      size: {
        default: 'h-8',
        sm: 'h-7 text-xs',
        lg: 'h-12 text-sm group-data-[collapsible=icon]:!p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export type SidebarMenuButtonVariants = VariantProps<typeof sidebarMenuButtonVariants>
