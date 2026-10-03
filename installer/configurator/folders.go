package main

import (
	"sort"
	"strings"
)

type folderOption struct {
	id    string
	label string
}

// buildFolderOptions generates a hierarchical list of folder options with visual tree indentation (↳ 📁).
// If includeAll is true, an initial "ALL" option is prepended.
func buildFolderOptions(folders []DeviceFolderItem, includeAll bool) []folderOption {
	options := make([]folderOption, 0, len(folders)+2)
	if includeAll {
		options = append(options, folderOption{id: "ALL", label: T("folder_all")})
	}
	options = append(options, folderOption{id: "", label: T("folder_root")})

	var addChildren func(parentID string, level int)
	addChildren = func(parentID string, level int) {
		var children []DeviceFolderItem
		for _, f := range folders {
			if f.ParentFolderID == parentID {
				children = append(children, f)
			}
		}
		sort.Slice(children, func(i, j int) bool {
			return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
		})

		for _, f := range children {
			prefix := "📁 "
			if level > 0 {
				prefix = strings.Repeat("  ", level) + "↳ 📁 "
			}
			options = append(options, folderOption{id: f.FolderID, label: prefix + f.Name})
			addChildren(f.FolderID, level+1)
		}
	}
	addChildren("", 0)
	return options
}

// buildFolderMoveOptions lists valid move destinations for a folder: the root
// plus every folder outside the moved subtree (a folder can never land
// inside itself or one of its descendants).
func buildFolderMoveOptions(folders []DeviceFolderItem, excludeID string) []folderOption {
	excluded := map[string]bool{}
	if excludeID != "" {
		excluded = getFolderAndDescendantIDs(excludeID, folders)
	}
	options := []folderOption{{id: "", label: T("folder_root")}}

	var addChildren func(parentID string, level int)
	addChildren = func(parentID string, level int) {
		var children []DeviceFolderItem
		for _, f := range folders {
			if f.ParentFolderID == parentID && !excluded[f.FolderID] {
				children = append(children, f)
			}
		}
		sort.Slice(children, func(i, j int) bool {
			return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
		})

		for _, f := range children {
			prefix := ""
			if level > 0 {
				prefix = strings.Repeat("  ", level) + "↳ "
			}
			options = append(options, folderOption{id: f.FolderID, label: prefix + f.Name})
			addChildren(f.FolderID, level+1)
		}
	}
	addChildren("", 0)
	return options
}

// getDeviceFolderName returns the display name for a folder ID.
func getDeviceFolderName(folderID string, folders []DeviceFolderItem) string {
	if folderID == "" || folderID == "ALL" {
		return ""
	}
	for _, f := range folders {
		if f.FolderID == folderID {
			return f.Name
		}
	}
	return ""
}

// getFolderAncestors returns the chain of ancestor folders from top-level down to the target folder.
// For example: [Siège Paris, Comptabilité, Serveurs]
func getFolderAncestors(folderID string, folders []DeviceFolderItem) []DeviceFolderItem {
	if folderID == "" || folderID == "ALL" {
		return nil
	}
	var ancestors []DeviceFolderItem
	curr := folderID
	visited := make(map[string]bool)
	for curr != "" && !visited[curr] {
		visited[curr] = true
		var found *DeviceFolderItem
		for i := range folders {
			if folders[i].FolderID == curr {
				found = &folders[i]
				break
			}
		}
		if found == nil {
			break
		}
		ancestors = append([]DeviceFolderItem{*found}, ancestors...)
		curr = found.ParentFolderID
	}
	return ancestors
}

// getFolderAndDescendantIDs returns a set containing folderID and all its recursive descendant IDs.
func getFolderAndDescendantIDs(folderID string, folders []DeviceFolderItem) map[string]bool {
	ids := map[string]bool{folderID: true}
	changed := true
	for changed {
		changed = false
		for _, f := range folders {
			if ids[f.ParentFolderID] && !ids[f.FolderID] {
				ids[f.FolderID] = true
				changed = true
			}
		}
	}
	return ids
}

// getFolderBreadcrumbPath returns a human-readable breadcrumb trail, e.g. "Siège Paris > Comptabilité > Serveurs".
func getFolderBreadcrumbPath(folderID string, folders []DeviceFolderItem) string {
	if folderID == "" || folderID == "ALL" {
		return ""
	}
	ancestors := getFolderAncestors(folderID, folders)
	if len(ancestors) == 0 {
		return getDeviceFolderName(folderID, folders)
	}
	names := make([]string, len(ancestors))
	for i, a := range ancestors {
		names[i] = a.Name
	}
	return strings.Join(names, " > ")
}

// getDirectChildFolders returns all immediate children of parentID, sorted alphabetically.
func getDirectChildFolders(parentID string, folders []DeviceFolderItem) []DeviceFolderItem {
	var children []DeviceFolderItem
	for _, f := range folders {
		if f.ParentFolderID == parentID {
			children = append(children, f)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
	})
	return children
}

// getFolderDepth returns the nesting level of the folder (1 = top-level, 2 = subfolder, 3 = level 3, etc.).
func getFolderDepth(folderID string, folders []DeviceFolderItem) int {
	return len(getFolderAncestors(folderID, folders))
}
