import type { TFunction } from 'i18next'

/** The name the server gives a tree created without one; must match store.DefaultTreeName. */
export const DEFAULT_TREE_NAME = 'Family tree'

/** The tree's name for display: the server's default name follows the interface language. */
export function treeDisplayName(name: string, t: TFunction): string {
  return name === DEFAULT_TREE_NAME ? t('auth.treeNamePlaceholder') : name
}
