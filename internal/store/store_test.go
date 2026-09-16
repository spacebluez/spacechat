package store

import (
	"path/filepath"
	"testing"
)

func TestPagesAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	instance := database.InstanceID()
	for index := 0; index < 205; index++ {
		if _, err = database.Append("小明", "你好"); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := database.LatestID()
	if err != nil || latest != 205 {
		t.Fatalf("latest %d %v", latest, err)
	}
	page, err := database.Page(0, 0, latest)
	if err != nil || len(page.Messages) != 100 || page.Messages[0].ID != 106 || !page.HasMore {
		t.Fatalf("latest page %+v %v", page, err)
	}
	page, err = database.Page(106, 0, latest)
	if err != nil || len(page.Messages) != 100 || page.Messages[0].ID != 6 || !page.HasMore {
		t.Fatalf("middle page %+v %v", page, err)
	}
	page, err = database.Page(6, 0, latest)
	if err != nil || len(page.Messages) != 5 || page.HasMore {
		t.Fatalf("first page %+v %v", page, err)
	}
	page, err = database.Page(0, 100, latest)
	if err != nil || len(page.Messages) != 100 || page.Messages[0].ID != 101 || !page.HasMore {
		t.Fatalf("sync page %+v %v", page, err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if instance == "" || instance != database.InstanceID() {
		t.Fatal("instance changed")
	}
	latest, err = database.LatestID()
	if err != nil || latest != 205 {
		t.Fatal("history lost")
	}
}
