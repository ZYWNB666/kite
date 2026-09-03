package model

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// UserGroup is a locally managed collection of users. Roles assigned to a
// local group are inherited by every current member.
type UserGroup struct {
	Model

	Name        string `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"`
	Description string `json:"description" gorm:"type:text"`
	Members     []User `json:"members" gorm:"many2many:user_group_members;constraint:OnDelete:CASCADE"`
}

var ErrInvalidGroupMember = errors.New("one or more group members do not exist")

func ListUserGroups() ([]UserGroup, error) {
	var groups []UserGroup
	err := DB.Preload("Members", "provider != ?", "api_key").Order("name asc").Find(&groups).Error
	return groups, err
}

func GetUserGroupByID(id uint) (*UserGroup, error) {
	var group UserGroup
	if err := DB.Preload("Members", "provider != ?", "api_key").First(&group, id).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func CreateUserGroup(group *UserGroup, memberIDs []uint) error {
	if group == nil {
		return errors.New("group is nil")
	}
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" {
		return errors.New("group name is required")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(group).Error; err != nil {
			return err
		}
		members, err := findGroupMembers(tx, memberIDs)
		if err != nil {
			return err
		}
		if len(members) > 0 {
			if err := tx.Model(group).Association("Members").Replace(members); err != nil {
				return fmt.Errorf("failed to set group members: %w", err)
			}
		}
		group.Members = members
		return nil
	})
}

func UpdateUserGroup(group *UserGroup, name, description string, memberIDs []uint) error {
	if group == nil {
		return errors.New("group is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("group name is required")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		members, err := findGroupMembers(tx, memberIDs)
		if err != nil {
			return err
		}
		oldName := group.Name
		group.Name = name
		group.Description = description
		if err := tx.Save(group).Error; err != nil {
			return err
		}
		if oldName != name {
			if err := tx.Model(&RoleAssignment{}).
				Where("subject_type = ? AND subject = ?", SubjectTypeLocalGroup, oldName).
				Update("subject", name).Error; err != nil {
				return fmt.Errorf("failed to update group role assignments: %w", err)
			}
		}
		if err := tx.Model(group).Association("Members").Replace(members); err != nil {
			return fmt.Errorf("failed to set group members: %w", err)
		}
		group.Members = members
		return nil
	})
}

func DeleteUserGroup(group *UserGroup) error {
	if group == nil {
		return errors.New("group is nil")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(group).Association("Members").Clear(); err != nil {
			return fmt.Errorf("failed to clear group members: %w", err)
		}
		if err := tx.Where("subject_type = ? AND subject = ?", SubjectTypeLocalGroup, group.Name).
			Delete(&RoleAssignment{}).Error; err != nil {
			return fmt.Errorf("failed to delete group role assignments: %w", err)
		}
		return tx.Delete(group).Error
	})
}

func findGroupMembers(tx *gorm.DB, memberIDs []uint) ([]User, error) {
	if len(memberIDs) == 0 {
		return []User{}, nil
	}
	uniqueIDs := make([]uint, 0, len(memberIDs))
	seen := make(map[uint]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		if id == 0 {
			return nil, ErrInvalidGroupMember
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	var members []User
	if err := tx.Where("id IN ? AND provider != ?", uniqueIDs, "api_key").Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) != len(uniqueIDs) {
		return nil, ErrInvalidGroupMember
	}
	return members, nil
}
