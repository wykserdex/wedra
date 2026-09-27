//go:build windows

package plugin

// Р­С‚РѕС‚ С„Р°Р№Р» РЅРµ СЂРµР°Р»РёР·СѓРµС‚ Windows-Р±СЌРєРµРЅРґ. РћРЅ С„РёРєСЃРёСЂСѓРµС‚ РёР·РјРµСЂРµРЅРёРµРј С‚Рѕ, С‡С‚Рѕ
// SECURITY.md С‚РµРїРµСЂСЊ СѓС‚РІРµСЂР¶РґР°РµС‚: Р·Р°РїРёСЃСЊ PROTECTED DACL РЅР° РєР°С‚Р°Р»РѕРі, РєРѕС‚РѕСЂС‹Р№
// РїСЂРѕС†РµСЃСЃ СЃРѕР·РґР°Р» СЃР°Рј, РЅРµРїСЂРёРІРёР»РµРіРёСЂРѕРІР°РЅРЅРѕРјСѓ РїСЂРѕС†РµСЃСЃСѓ РґРѕСЃС‚СѓРїРЅР°, Рё РѕРЅР° СЂРµР°Р»СЊРЅРѕ
// РѕС‚СЂРµР·Р°РµС‚ РµРіРѕ СЃРѕР±СЃС‚РІРµРЅРЅС‹Р№ С‚РѕРєРµРЅ. Р Р°РЅСЊС€Рµ Р·РґРµСЃСЊ СЃС‚РѕСЏР»Рѕ В«host ACL stateВ» РєР°Рє
// Р±Р»РѕРєРµСЂ вЂ” РёР·РјРµСЂРµРЅРёРµ РїРѕРєР°Р·Р°Р»Рѕ, С‡С‚Рѕ Р±Р»РѕРєРµСЂ РґСЂСѓРіРѕР№ (СѓСЂРѕРІРµРЅСЊ С†РµР»РѕСЃС‚РЅРѕСЃС‚Рё), Р°
// СЃР°Рј DACL СЂР°Р±РѕС‚Р°РµС‚. РЈС‚РІРµСЂР¶РґРµРЅРёРµ Р±РµР· С‚РµСЃС‚Р° СЃРЅРѕРІР° РјРѕР¶РµС‚ СѓСЃС‚Р°СЂРµС‚СЊ.

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"wedra/internal/pipeline"
)

// allApplicationPackages вЂ” S-1-15-2, SID, РїРѕРґ РєРѕС‚РѕСЂС‹Рј С…РѕРґСЏС‚ РІСЃРµ AppContainer-С‚РѕРєРµРЅС‹.
const allApplicationPackages = "S-1-15-2"

func tHelperSID(t *testing.T, wellKnown func() (*windows.SID, error)) *windows.SID {
	t.Helper()
	sid, err := wellKnown()
	if err != nil {
		t.Skipf("SID РЅРµРґРѕСЃС‚СѓРїРµРЅ: %v", err)
	}
	return sid
}

func grantAce(sid *windows.SID) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       0,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

// setDACL вЂ” СЃС‚Р°РІРёС‚ DACL, РїРѕ СѓРјРѕР»С‡Р°РЅРёСЋ СЂРІС‘С‚ РЅР°СЃР»РµРґРѕРІР°РЅРёРµ (PROTECTED).
func setDACL(t *testing.T, path string, protect bool, sids ...*windows.SID) error {
	t.Helper()
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(sids))
	for _, s := range sids {
		entries = append(entries, grantAce(s))
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	si := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION)
	if protect {
		si |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		si |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, si, nil, nil, acl, nil)
}

// currentUserSID вЂ” SID РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ СЌС‚РѕРіРѕ РїСЂРѕС†РµСЃСЃР°.
func currentUserSID(t *testing.T) *windows.SID {
	t.Helper()
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || tu == nil || tu.User.Sid == nil {
		t.Skipf("SID РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ РЅРµРґРѕСЃС‚СѓРїРµРЅ: %v", err)
	}
	return tu.User.Sid
}

// TestWindowsSelfCreatedDirDACLIsRestrictable вЂ” С†РµРЅС‚СЂР°Р»СЊРЅРѕРµ СѓС‚РІРµСЂР¶РґРµРЅРёРµ
// SECURITY.md. РќРµРїСЂРёРІРёР»РµРіРёСЂРѕРІР°РЅРЅС‹Р№ РїСЂРѕС†РµСЃСЃ РѕР±СЏР·Р°РЅ СЃСѓР·РёС‚СЊ DACL СЃРѕР±СЃС‚РІРµРЅРЅРѕРіРѕ
// РєР°С‚Р°Р»РѕРіР° Рё РѕС‚СЂРµР·Р°С‚СЊ СЃРµР±Рµ Р¶Рµ РґРѕСЃС‚СѓРї.
func TestWindowsSelfCreatedDirDACLIsRestrictable(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sandbox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	sys := tHelperSID(t, func() (*windows.SID, error) { return windows.CreateWellKnownSid(windows.WinLocalSystemSid) })
	adm := tHelperSID(t, func() (*windows.SID, error) {
		return windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	})
	app, err := windows.StringToSid(allApplicationPackages)
	if err != nil {
		t.Skipf("S-1-15-2 РЅРµРґРѕСЃС‚СѓРїРµРЅ: %v", err)
	}

	user := currentUserSID(t)

	// Р’РѕСЃСЃС‚Р°РЅРѕРІР»РµРЅРёРµ РѕР±СЏР·Р°С‚РµР»СЊРЅРѕ: Р±РµР· РЅРµРіРѕ СѓРїР°РІС€РёР№ С‚РµСЃС‚ РѕСЃС‚Р°РІРёР» Р±С‹ РєР°С‚Р°Р»РѕРі
	// Р·Р°РєСЂС‹С‚С‹Рј, Р° СЌС‚Рѕ СѓР¶Рµ Р»РѕРјР°РµС‚ РјР°С€РёРЅСѓ, РЅР° РєРѕС‚РѕСЂРѕР№ С‚РµСЃС‚ Р·Р°РїСѓС‰РµРЅ.
	defer func() {
		back := []*windows.SID{sys, adm}
		if user != nil {
			back = append(back, user)
		}
		_ = setDACL(t, dir, false, back...)
	}()

	// Р”Рѕ РїСЂР°РІРєРё РґРѕСЃС‚СѓРї РґРѕР»Р¶РµРЅ Р±С‹С‚СЊ.
	if _, err := os.ReadDir(dir); err != nil {
		t.Fatalf("РґРѕ Р·Р°РїРёСЃРё DACL РєР°С‚Р°Р»РѕРі РґРѕР»Р¶РµРЅ Р±С‹С‚СЊ РґРѕСЃС‚СѓРїРµРЅ: %v", err)
	}

	// PROTECTED вЂ” РЅРµСЃСѓС‰РёР№ С„Р»Р°Рі: РѕРЅ СЂРІС‘С‚ РЅР°СЃР»РµРґРѕРІР°РЅРёРµ.
	if err := setDACL(t, dir, true, sys, adm, app); err != nil {
		t.Fatalf("SetNamedSecurityInfo РЅР° СЃРѕР±СЃС‚РІРµРЅРЅРѕРј РєР°С‚Р°Р»РѕРіРµ РѕР±СЏР·Р°РЅ СЂР°Р±РѕС‚Р°С‚СЊ Р±РµР· РїРѕРІС‹С€РµРЅРёСЏ, РїРѕР»СѓС‡РµРЅРѕ: %v", err)
	}

	// РџСЂРѕРІРµСЂСЏРµРј СЃС‚СЂСѓРєС‚СѓСЂРЅРѕ, Р° РЅРµ В«ReadDir РїР°РґР°РµС‚В». Р“СЂСѓРїРїРµ Administrators
	// РІС‹РґР°РЅ GENERIC_ALL, РїРѕСЌС‚РѕРјСѓ Сѓ РїСЂРѕС†РµСЃСЃР°, РєРѕС‚РѕСЂС‹Р№ СЃР°Рј СЃРѕСЃС‚РѕРёС‚ РІ Р°РґРјРёРЅР°С… Рё
	// СЂР°Р±РѕС‚Р°РµС‚ СЃ РїРѕРІС‹С€РµРЅРЅС‹Рј С‚РѕРєРµРЅРѕРј, РґРѕСЃС‚СѓРї Р·Р°РєРѕРЅРЅРѕ РѕСЃС‚Р°С‘С‚СЃСЏ С‡РµСЂРµР· СЌС‚РѕС‚ ACE вЂ”
	// С‚Р°Рє Рё РґРѕР»Р¶РЅРѕ Р±С‹С‚СЊ, РїРѕРІС‹С€РµРЅРЅС‹Р№ РїСЂРѕС†РµСЃСЃ РІ Р»СЋР±РѕРј СЃР»СѓС‡Р°Рµ РјРѕР¶РµС‚ РІРµСЂРЅСѓС‚СЊ
	// РєР°С‚Р°Р»РѕРі СЃРµР±Рµ. Р‘Р»РѕРєРёСЂСѓРµС‚СЃСЏ РёРјРµРЅРЅРѕ РЅРµРїСЂРёРІРёР»РµРіРёСЂРѕРІР°РЅРЅС‹Р№ С‚РѕРєРµРЅ: Сѓ РЅРµРіРѕ SID
	// Administrators РІ deny-only, Рё С‚РѕРіРґР° РѕСЃС‚Р°С‘С‚СЃСЏ С‚РѕР»СЊРєРѕ SYSTEM/Administrators/
	// S-1-15-2, Р° РїРѕР»СЊР·РѕРІР°С‚РµР»СЊСЃРєРѕРіРѕ SID РІ DACL РЅРµС‚ РІРѕРІСЃРµ.
	if aclGrants(t, dir, user) {
		t.Fatal("РїРѕСЃР»Рµ PROTECTED DACL SID РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ РЅРµ РґРѕР»Р¶РµРЅ РѕСЃС‚Р°С‚СЊСЃСЏ РІ DACL")
	}

	if !elevated() {
		if _, err := os.ReadDir(dir); err == nil {
			t.Error("РЅРµРїСЂРёРІРёР»РµРіРёСЂРѕРІР°РЅРЅС‹Р№ С‚РѕРєРµРЅ РґРѕР»Р¶РµРЅ Р±С‹С‚СЊ РѕС‚СЂРµР·Р°РЅ, РЅРѕ РґРѕСЃС‚СѓРї СЃРѕС…СЂР°РЅРёР»СЃСЏ")
		}
	}
}

// aclGrants вЂ” РµСЃС‚СЊ Р»Рё РІ DACL РїСѓС‚Рё РїСЂСЏРјРѕР№ РІС‹РґР°С‡Рё СѓРєР°Р·Р°РЅРЅРѕРјСѓ SID.
func aclGrants(t *testing.T, path string, sid *windows.SID) bool {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo: %v", err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return false
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			continue
		}
		// SID в ACCESS_ALLOWED_ACE начинается сразу за маской, то есть по
		// адресу SidStart. Заголовок SID и ACCESS_ALLOWED_ACE совместимы по
		// раскладке, поэтому указатель кастуется напрямую.
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if aceSID == nil || aceSID.SubAuthorityCount() > 15 {
			continue
		}
		if aceSID.String() == sid.String() {
			return true
		}
	}
	return false
}

func elevated() bool {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok); err != nil {
		return false
	}
	defer tok.Close()
	return tok.IsElevated()
}

// TestWindowsDACLWithoutProtectedKeepsUserAccess вЂ” РєРѕРЅС‚СЂРѕР»СЊ: С„Р»Р°Рі
// PROTECTED_DACL_SECURITY_INFORMATION РЅРµСЃСѓС‰РёР№. Р‘РµР· РЅРµРіРѕ РЅРѕРІС‹Р№ ACL СЃР»РёРІР°РµС‚СЃСЏ СЃ
// СѓРЅР°СЃР»РµРґРѕРІР°РЅРЅС‹РјРё ACE, Рё РѕРіСЂР°РЅРёС‡РµРЅРёСЏ РЅРµ РІРѕР·РЅРёРєР°РµС‚.
func TestWindowsDACLWithoutProtectedKeepsUserAccess(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sandbox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	sys := tHelperSID(t, func() (*windows.SID, error) { return windows.CreateWellKnownSid(windows.WinLocalSystemSid) })
	adm := tHelperSID(t, func() (*windows.SID, error) {
		return windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	})
	app, err := windows.StringToSid(allApplicationPackages)
	if err != nil {
		t.Skipf("S-1-15-2 РЅРµРґРѕСЃС‚СѓРїРµРЅ: %v", err)
	}
	user := currentUserSID(t)
	defer func() {
		back := []*windows.SID{sys, adm}
		if user != nil {
			back = append(back, user)
		}
		_ = setDACL(t, dir, false, back...)
	}()

	if err := setDACL(t, dir, false, sys, adm, app); err != nil {
		t.Fatalf("SetNamedSecurityInfo Р±РµР· PROTECTED: %v", err)
	}
	if _, err := os.ReadDir(dir); err != nil {
		t.Fatalf("Р±РµР· PROTECTED РЅР°СЃР»РµРґРѕРІР°РЅРёРµ РґРѕР»Р¶РЅРѕ СЃРѕС…СЂР°РЅРёС‚СЊ РґРѕСЃС‚СѓРї РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ, РїРѕР»СѓС‡РµРЅРѕ: %v", err)
	}
}

// TestWindowsNoWindowsBackendRemainsFailClosed вЂ” РёР·РѕР»СЏС†РёСЏ РЅР° Windows РЅРµ
// РїРѕСЏРІРёР»Р°СЃСЊ: СЌС‚РѕС‚ С‚РµСЃС‚ С„РёРєСЃРёСЂСѓРµС‚, С‡С‚Рѕ Р±СЌРєРµРЅРґР° РЅРµС‚ Рё СЏРґСЂРѕ РѕР±СЏР·Р°РЅРѕ РѕС‚РєР°Р·Р°С‚СЊ.
// Р•СЃР»Рё РєРѕРіРґР°-РЅРёР±СѓРґСЊ РїРѕСЏРІРёС‚СЃСЏ РЅР°СЃС‚РѕСЏС‰РёР№ AppContainer-Р±СЌРєРµРЅРґ, С‚РµСЃС‚ РЅР°РїРѕРјРЅРёС‚
// РѕР±РЅРѕРІРёС‚СЊ SECURITY.md.
func TestWindowsNoWindowsBackendRemainsFailClosed(t *testing.T) {
	if _, ok := sandboxBackend(); ok {
		t.Skip("Windows-Р±СЌРєРµРЅРґ РїРѕСЏРІРёР»СЃСЏ вЂ” РѕР±РЅРѕРІРёС‚Рµ SECURITY.md Рё СЌС‚РѕС‚ С‚РµСЃС‚")
	}
	m := &pipeline.Manifest{
		ID:      "x",
		Runtime: pipeline.Runtime{Type: "python", Entry: "plugin.py"},
		Dir:     t.TempDir(),
	}
	if _, _, err := sandboxArgs(m, []string{"/bin/sh", "-c", "true"}, t.TempDir()); err == nil {
		t.Fatal("Р±РµР· Windows-Р±СЌРєРµРЅРґР° Р·Р°РїСѓСЃРє РІРЅРµС€РЅРµРіРѕ РєРѕРґР° РѕР±СЏР·Р°РЅ Р±С‹С‚СЊ РѕС‚РєР°Р·, Р° РЅРµ СЂР°Р·СЂРµС€С‘РЅ")
	}
}
