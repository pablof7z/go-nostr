package relay

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip29"
)

type Action interface {
	Apply(group *nip29.Group)
	PermissionName() nip29.Permission
}

func GetModerationAction(evt *nostr.Event) (Action, error) {
	factory, ok := moderationActionFactories[evt.Kind]
	if !ok {
		return nil, fmt.Errorf("event kind %d is not a supported moderation action", evt.Kind)
	}
	return factory(evt)
}

var moderationActionFactories = map[int]func(*nostr.Event) (Action, error){
	nostr.KindSimpleGroupAddUser: func(evt *nostr.Event) (Action, error) {
		targets := make([]string, 0, len(evt.Tags))
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValid32ByteHex(tag[1]) {
				return nil, fmt.Errorf("")
			}
			targets = append(targets, tag[1])
		}
		if len(targets) > 0 {
			return &AddUser{Targets: targets}, nil
		}
		return nil, fmt.Errorf("missing 'p' tags")
	},
	nostr.KindSimpleGroupRemoveUser: func(evt *nostr.Event) (Action, error) {
		targets := make([]string, 0, len(evt.Tags))
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValid32ByteHex(tag[1]) {
				return nil, fmt.Errorf("invalid public key hex")
			}
			targets = append(targets, tag[1])
		}
		if len(targets) > 0 {
			return &RemoveUser{Targets: targets}, nil
		}
		return nil, fmt.Errorf("missing 'p' tags")
	},
	nostr.KindSimpleGroupEditMetadata: func(evt *nostr.Event) (Action, error) {
		ok := false
		edit := EditMetadata{When: evt.CreatedAt}
		if t := evt.Tags.GetFirst([]string{"name", ""}); t != nil {
			edit.NameValue = (*t)[1]
			ok = true
		}
		if t := evt.Tags.GetFirst([]string{"picture", ""}); t != nil {
			edit.PictureValue = (*t)[1]
			ok = true
		}
		if t := evt.Tags.GetFirst([]string{"about", ""}); t != nil {
			edit.AboutValue = (*t)[1]
			ok = true
		}
		if ok {
			return &edit, nil
		}
		return nil, fmt.Errorf("missing metadata tags")
	},
	nostr.KindSimpleGroupAddPermission: func(evt *nostr.Event) (Action, error) {
		nTags := len(evt.Tags)

		permissions := make([]nip29.Permission, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"permission", ""}) {
			perm := nip29.Permission(tag[1])
			if _, ok := nip29.PermissionsMap[perm]; !ok {
				return nil, fmt.Errorf("unknown permission '%s'", tag[1])
			}
			permissions = append(permissions, perm)
		}

		targets := make([]string, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValid32ByteHex(tag[1]) {
				return nil, fmt.Errorf("invalid public key hex")
			}
			targets = append(targets, tag[1])
		}

		if len(permissions) > 0 && len(targets) > 0 {
			return &AddPermission{Initiator: evt.PubKey, Targets: targets, Permissions: permissions}, nil
		}

		return nil, fmt.Errorf("")
	},
	nostr.KindSimpleGroupRemovePermission: func(evt *nostr.Event) (Action, error) {
		nTags := len(evt.Tags)

		permissions := make([]nip29.Permission, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"permission", ""}) {
			perm := nip29.Permission(tag[1])
			if _, ok := nip29.PermissionsMap[perm]; !ok {
				return nil, fmt.Errorf("unknown permission '%s'", tag[1])
			}
			permissions = append(permissions, perm)
		}

		targets := make([]string, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValid32ByteHex(tag[1]) {
				return nil, fmt.Errorf("invalid public key hex")
			}
			targets = append(targets, tag[1])
		}

		if len(permissions) > 0 && len(targets) > 0 {
			return &RemovePermission{Targets: targets, Permissions: permissions}, nil
		}

		return nil, fmt.Errorf("")
	},
	nostr.KindSimpleGroupDeleteEvent: func(evt *nostr.Event) (Action, error) {
		tags := evt.Tags.GetAll([]string{"e", ""})
		if len(tags) == 0 {
			return nil, fmt.Errorf("missing 'e' tag")
		}

		targets := make([]string, len(tags))
		for i, tag := range tags {
			if nostr.IsValid32ByteHex(tag[1]) {
				targets[i] = tag[1]
			} else {
				return nil, fmt.Errorf("invalid event id hex")
			}
		}

		return &DeleteEvent{Targets: targets}, nil
	},
	nostr.KindSimpleGroupEditGroupStatus: func(evt *nostr.Event) (Action, error) {
		egs := EditGroupStatus{When: evt.CreatedAt}

		egs.Public = evt.Tags.GetFirst([]string{"public"}) != nil
		egs.Private = evt.Tags.GetFirst([]string{"private"}) != nil
		egs.Open = evt.Tags.GetFirst([]string{"open"}) != nil
		egs.Closed = evt.Tags.GetFirst([]string{"closed"}) != nil

		// disallow contradictions
		if egs.Public && egs.Private {
			return nil, fmt.Errorf("contradiction")
		}
		if egs.Open && egs.Closed {
			return nil, fmt.Errorf("contradiction")
		}

		return egs, nil
	},
}

type DeleteEvent struct {
	Targets []string
}

func (DeleteEvent) PermissionName() nip29.Permission { return nip29.PermDeleteEvent }
func (a DeleteEvent) Apply(group *nip29.Group)       {}

type AddUser struct {
	Targets []string
}

func (AddUser) PermissionName() nip29.Permission { return nip29.PermAddUser }
func (a AddUser) Apply(group *nip29.Group) {
	for _, target := range a.Targets {
		group.Members[target] = nip29.EmptyRole
	}
}

type RemoveUser struct {
	Targets []string
}

func (RemoveUser) PermissionName() nip29.Permission { return nip29.PermRemoveUser }
func (a RemoveUser) Apply(group *nip29.Group) {
	for _, tpk := range a.Targets {
		if target, ok := group.Members[tpk]; ok {
			if target != nip29.EmptyRole {
				_, hasSuperiorOrEqualPermission := target.Permissions[nip29.PermRemoveUser]
				if hasSuperiorOrEqualPermission {
					continue
				}
			}
			delete(group.Members, tpk)
		}
	}
}

type EditMetadata struct {
	NameValue    string
	PictureValue string
	AboutValue   string
	When         nostr.Timestamp
}

func (EditMetadata) PermissionName() nip29.Permission { return nip29.PermEditMetadata }
func (a EditMetadata) Apply(group *nip29.Group) {
	group.Name = a.NameValue
	group.Picture = a.PictureValue
	group.About = a.AboutValue
	group.LastMetadataUpdate = a.When
}

type AddPermission struct {
	Initiator   string // the user who is adding the permissions
	Targets     []string
	Permissions []nip29.Permission
}

func (AddPermission) PermissionName() nip29.Permission { return nip29.PermAddPermission }
func (a AddPermission) Apply(group *nip29.Group) {
	for _, tpk := range a.Targets {
		target, ok := group.Members[tpk]

		// if it's a normal user, create a new permissions object thing for this user
		// instead of modifying the global EmptyRole
		if !ok || target == nip29.EmptyRole {
			target = &nip29.Role{Permissions: make(map[nip29.Permission]struct{})}
			group.Members[tpk] = target
		}

		// only add permissions that the user performing this already have
		initiator, ok := group.Members[a.Initiator]
		if ok {
			for _, perm := range a.Permissions {
				if _, has := initiator.Permissions[perm]; has {
					target.Permissions[perm] = struct{}{}
				}
			}
		}
	}
}

type RemovePermission struct {
	Targets     []string
	Permissions []nip29.Permission
}

func (RemovePermission) PermissionName() nip29.Permission { return nip29.PermRemovePermission }
func (a RemovePermission) Apply(group *nip29.Group) {
	for _, tpk := range a.Targets {
		target, ok := group.Members[tpk]
		if !ok || target == nip29.EmptyRole {
			continue
		}

		_, hasSuperiorOrEqualPermission := target.Permissions[nip29.PermRemovePermission]
		if hasSuperiorOrEqualPermission {
			continue
		}

		// remove all permissions listed
		for _, perm := range a.Permissions {
			delete(target.Permissions, perm)
		}

		// if no more permissions are available, change this guy to be a normal user
		if target.Name == "" && len(target.Permissions) == 0 {
			group.Members[tpk] = nip29.EmptyRole
		}
	}
}

type EditGroupStatus struct {
	Public  bool
	Private bool
	Open    bool
	Closed  bool
	When    nostr.Timestamp
}

func (EditGroupStatus) PermissionName() nip29.Permission { return nip29.PermEditGroupStatus }
func (a EditGroupStatus) Apply(group *nip29.Group) {
	if a.Public {
		group.Private = false
	} else if a.Private {
		group.Private = true
	}

	if a.Open {
		group.Closed = false
	} else if a.Closed {
		group.Closed = true
	}

	group.LastMetadataUpdate = a.When
}
