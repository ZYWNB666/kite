import { useEffect, useMemo, useState } from 'react'
import {
  IconEdit,
  IconPlus,
  IconShieldCheck,
  IconTrash,
  IconUsersGroup,
} from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { UserGroup, UserItem } from '@/types/api'
import {
  createUserGroup,
  deleteUserGroup,
  updateUserGroup,
  UserGroupRequest,
  useRoleList,
  useUserGroupList,
  useUserList,
} from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Textarea } from '@/components/ui/textarea'
import { Action, ActionTable } from '@/components/action-table'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

import { UserRoleAssignment } from './user-role-assignment'

interface GroupDialogProps {
  open: boolean
  group: UserGroup | null
  users: UserItem[]
  saving: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (data: UserGroupRequest) => void
}

function GroupDialog({
  open,
  group,
  users,
  saving,
  onOpenChange,
  onSubmit,
}: GroupDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [search, setSearch] = useState('')
  const [memberIDs, setMemberIDs] = useState<Set<number>>(new Set())

  useEffect(() => {
    if (!open) return
    setName(group?.name || '')
    setDescription(group?.description || '')
    setSearch('')
    setMemberIDs(new Set(group?.members?.map((member) => member.id) || []))
  }, [group, open])

  const filteredUsers = useMemo(() => {
    const needle = search.trim().toLowerCase()
    if (!needle) return users
    return users.filter(
      (user) =>
        user.username.toLowerCase().includes(needle) ||
        user.name?.toLowerCase().includes(needle)
    )
  }, [search, users])

  const toggleMember = (id: number) => {
    setMemberIDs((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault()
    if (!name.trim()) return
    onSubmit({
      name: name.trim(),
      description: description.trim(),
      memberIds: Array.from(memberIDs),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>
            {group
              ? t('groupManagement.dialog.editTitle', 'Edit User Group')
              : t('groupManagement.dialog.addTitle', 'Add User Group')}
          </DialogTitle>
          <DialogDescription>
            {group
              ? t(
                  'groupManagement.dialog.editDescription',
                  'Update the group details and membership.'
                )
              : t(
                  'groupManagement.dialog.addDescription',
                  'Create a group and choose its initial members.'
                )}
          </DialogDescription>
        </DialogHeader>
        <form
          id="user-group-form"
          onSubmit={handleSubmit}
          className="min-h-0 flex-1 overflow-y-auto space-y-4 pr-1"
        >
          <div className="space-y-2">
            <Label htmlFor="user-group-name">
              {t('common.name', 'Name')} *
            </Label>
            <Input
              id="user-group-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={100}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="user-group-description">
              {t('common.description', 'Description')}
            </Label>
            <Textarea
              id="user-group-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              rows={3}
            />
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between gap-3">
              <Label>{t('groupManagement.members', 'Members')}</Label>
              <span className="text-xs text-muted-foreground">
                {memberIDs.size} {t('groupManagement.selected', 'selected')}
              </span>
            </div>
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('groupManagement.searchUsers', 'Search users...')}
            />
            <div className="max-h-60 overflow-y-auto rounded-md border">
              {filteredUsers.map((user) => (
                <label
                  key={user.id}
                  className="flex cursor-pointer items-center gap-3 border-b px-3 py-2.5 last:border-b-0 hover:bg-muted/50"
                >
                  <Checkbox
                    checked={memberIDs.has(user.id)}
                    onCheckedChange={() => toggleMember(user.id)}
                  />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">
                      {user.name || user.username}
                    </span>
                    {user.name && (
                      <span className="block truncate text-xs text-muted-foreground">
                        {user.username}
                      </span>
                    )}
                  </span>
                </label>
              ))}
              {filteredUsers.length === 0 && (
                <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                  {t('groupManagement.noUsers', 'No users found')}
                </div>
              )}
            </div>
          </div>
        </form>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('common.cancel', 'Cancel')}
          </Button>
          <Button
            type="submit"
            form="user-group-form"
            disabled={!name.trim() || saving}
          >
            {t('common.save', 'Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function UserGroupManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: groups = [], isLoading, error } = useUserGroupList()
  const { data: usersData } = useUserList(1, 1000)
  const { data: roles = [] } = useRoleList()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingGroup, setEditingGroup] = useState<UserGroup | null>(null)
  const [deletingGroup, setDeletingGroup] = useState<UserGroup | null>(null)
  const [assigningGroup, setAssigningGroup] = useState<UserGroup | null>(null)

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['user-group-list'] })
    queryClient.invalidateQueries({ queryKey: ['user-list'] })
    queryClient.invalidateQueries({ queryKey: ['role-list'] })
  }

  const createMutation = useMutation({
    mutationFn: createUserGroup,
    onSuccess: () => {
      refresh()
      setDialogOpen(false)
      toast.success(t('groupManagement.messages.created', 'User group created'))
    },
    onError: (mutationError: Error) => toast.error(mutationError.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: UserGroupRequest }) =>
      updateUserGroup(id, data),
    onSuccess: () => {
      refresh()
      setDialogOpen(false)
      setEditingGroup(null)
      toast.success(t('groupManagement.messages.updated', 'User group updated'))
    },
    onError: (mutationError: Error) => toast.error(mutationError.message),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteUserGroup,
    onSuccess: () => {
      refresh()
      setDeletingGroup(null)
      toast.success(t('groupManagement.messages.deleted', 'User group deleted'))
    },
    onError: (mutationError: Error) => toast.error(mutationError.message),
  })

  const columns = useMemo<ColumnDef<UserGroup>[]>(
    () => [
      {
        id: 'name',
        header: t('common.name', 'Name'),
        cell: ({ row: { original: group } }) => (
          <div>
            <div className="font-medium">{group.name}</div>
            {group.description && (
              <div className="text-sm text-muted-foreground">
                {group.description}
              </div>
            )}
          </div>
        ),
      },
      {
        id: 'members',
        header: t('groupManagement.members', 'Members'),
        cell: ({ row: { original: group } }) => (
          <div className="flex max-w-sm flex-wrap gap-1">
            {group.members?.slice(0, 4).map((member) => (
              <Badge key={member.id} variant="secondary">
                {member.name || member.username}
              </Badge>
            ))}
            {group.members && group.members.length > 4 && (
              <Popover>
                <PopoverTrigger asChild>
                  <button
                    type="button"
                    aria-label={t(
                      'groupManagement.viewMembers',
                      'View all members'
                    )}
                    className="rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <Badge
                      variant="outline"
                      className="cursor-pointer transition-colors hover:border-primary hover:text-primary"
                    >
                      +{group.members.length - 4}
                    </Badge>
                  </button>
                </PopoverTrigger>
                <PopoverContent className="w-56 p-2" align="start">
                  <div className="px-2 pb-1.5 text-xs font-semibold text-muted-foreground">
                    {t('groupManagement.members', 'Members')} ·{' '}
                    {group.members.length}
                  </div>
                  <div className="max-h-64 overflow-y-auto">
                    {group.members.map((member) => (
                      <div
                        key={member.id}
                        className="rounded px-2 py-1.5 text-sm hover:bg-muted/50"
                      >
                        <span className="block truncate font-medium">
                          {member.name || member.username}
                        </span>
                        {member.name && (
                          <span className="block truncate text-xs text-muted-foreground">
                            {member.username}
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
                </PopoverContent>
              </Popover>
            )}
            {!group.members?.length && (
              <span className="text-sm text-muted-foreground">-</span>
            )}
          </div>
        ),
      },
      {
        id: 'roles',
        header: t('groupManagement.roles', 'Roles'),
        cell: ({ row: { original: group } }) => {
          const assignedRoles = roles.filter((role) =>
            role.assignments?.some(
              (assignment) =>
                assignment.subjectType === 'local_group' &&
                assignment.subject === group.name
            )
          )
          return (
            <div className="flex flex-wrap gap-1">
              {assignedRoles.map((role) => (
                <Badge key={role.id} variant="outline">
                  {role.name}
                </Badge>
              ))}
              {assignedRoles.length === 0 && (
                <span className="text-sm text-muted-foreground">-</span>
              )}
            </div>
          )
        },
      },
    ],
    [roles, t]
  )

  const actions = useMemo<Action<UserGroup>[]>(
    () => [
      {
        label: (
          <>
            <IconShieldCheck className="h-4 w-4" />
            {t('common.assign', 'Assign')}
          </>
        ),
        onClick: setAssigningGroup,
      },
      {
        label: (
          <>
            <IconEdit className="h-4 w-4" />
            {t('common.edit', 'Edit')}
          </>
        ),
        onClick: (group) => {
          setEditingGroup(group)
          setDialogOpen(true)
        },
      },
      {
        label: (
          <span className="inline-flex items-center gap-2 text-destructive">
            <IconTrash className="h-4 w-4" />
            {t('common.delete', 'Delete')}
          </span>
        ),
        onClick: setDeletingGroup,
      },
    ],
    [t]
  )

  const handleSubmit = (data: UserGroupRequest) => {
    if (editingGroup) {
      updateMutation.mutate({ id: editingGroup.id, data })
    } else {
      createMutation.mutate(data)
    }
  }

  if (isLoading) {
    return (
      <div className="py-8 text-center text-muted-foreground">
        {t('common.loading', 'Loading...')}
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-8 text-center text-destructive">
        {t('groupManagement.errors.loadFailed', 'Failed to load user groups')}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-4">
            <CardTitle className="flex items-center gap-2">
              <IconUsersGroup className="h-5 w-5" />
              {t('groupManagement.title', 'User Group Management')}
            </CardTitle>
            <Button
              className="gap-2"
              onClick={() => {
                setEditingGroup(null)
                setDialogOpen(true)
              }}
            >
              <IconPlus className="h-4 w-4" />
              {t('groupManagement.actions.add', 'Add Group')}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <ActionTable actions={actions} data={groups} columns={columns} />
          {groups.length === 0 && (
            <div className="py-8 text-center text-muted-foreground">
              <IconUsersGroup className="mx-auto mb-4 h-12 w-12 opacity-50" />
              <p>{t('groupManagement.empty.title', 'No user groups')}</p>
              <p className="mt-1 text-sm">
                {t(
                  'groupManagement.empty.description',
                  'Create a group to manage permissions for multiple users'
                )}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <GroupDialog
        open={dialogOpen}
        group={editingGroup}
        users={usersData?.users || []}
        saving={createMutation.isPending || updateMutation.isPending}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) setEditingGroup(null)
        }}
        onSubmit={handleSubmit}
      />

      <UserRoleAssignment
        open={!!assigningGroup}
        onOpenChange={(open) => {
          if (!open) setAssigningGroup(null)
        }}
        subject={
          assigningGroup
            ? { type: 'local_group', name: assigningGroup.name }
            : undefined
        }
      />

      <DeleteConfirmationDialog
        open={!!deletingGroup}
        onOpenChange={(open) => {
          if (!open) setDeletingGroup(null)
        }}
        onConfirm={() => {
          if (deletingGroup) deleteMutation.mutate(deletingGroup.id)
        }}
        resourceName={deletingGroup?.name || ''}
        resourceType="user group"
      />
    </div>
  )
}

export default UserGroupManagement
