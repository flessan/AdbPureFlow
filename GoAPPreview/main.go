package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const adbURL = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
const engineDir = "adb_engine"

func main() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("===========================================")
	fmt.Println("             GoAPPreview v1 CLI            ")
	fmt.Println("===========================================")

	// 1. Setup ADB
	adbPath := setupADB()
	if adbPath == "" {
		fmt.Println("[!] Gagal menyiapkan ADB. Cek koneksi internet.\nFailed to set up ADB. Check your internet connection.")
		tungguEnter(reader)
		return
	}

	// 2. Input APK
	fmt.Print("\n[1] Tarik file APK ke sini & Enter:\nDrag the APK file here & Enter: ")
	apkPathRaw, _ := reader.ReadString('\n')
	apkPath := strings.TrimSpace(strings.Trim(apkPathRaw, "\"\r\n"))
	if apkPath == "" {
		return
	}

	// 3. Scan Sebelum Install
	fmt.Println("[2] Memindai daftar aplikasi di HP...\n[2] Scanning list of applications on your device...")
	beforeList := getPackageList(adbPath)

	// 4. Install
	fmt.Println("[3] Memasang aplikasi ke HP...\n[3] Installing the application to your device...")
	installCmd := exec.Command(adbPath, "install", "-r", apkPath)
	installCmd.Stdout = os.Stdout
	installCmd.Run()

	// 5. Scan Sesudah & Identifikasi
	fmt.Println("[4] Mencari ID aplikasi baru...\n[4] Searching for new application ID...")
	afterList := getPackageList(adbPath)
	packageName := findNewPackage(beforeList, afterList)

	if packageName == "" {
		fmt.Println("[!] Tidak ada ID baru. Aplikasi mungkin sudah ada atau gagal terpasang.\n[!] No new ID found. The application may already exist or failed to install.")
		fmt.Print("Ketik ID manual (contoh: com.example.app) atau Enter untuk batal: \nType the manual ID (example: com.example.app) or press Enter to cancel: ")
		manual, _ := reader.ReadString('\n')
		packageName = strings.TrimSpace(manual)
		if packageName == "" {
			return
		}
	}

	fmt.Printf("\nSUCCESS! Terdeteksi: %s\n", packageName)

	// 6. Auto-Launch (Fitur Baru)
	fmt.Println("[5] Menunggu sistem... membuka aplikasi otomatis...\n[5] Waiting for the system... opening the application automatically...")
	time.Sleep(1500 * time.Millisecond) // Jeda 1.5 detik agar Android siap
	exec.Command(adbPath, "shell", "monkey", "-p", packageName, "-c", "android.intent.category.LAUNCHER", "1").Run()

	// 7. Menu Konfirmasi & Verifikasi Uninstall
	fmt.Println("\n===========================================")
	fmt.Println("      APLIKASI AKTIF DI PERANGKAT       ")
	fmt.Println("===========================================")
	fmt.Print("Hapus aplikasi sekarang? (y/n): \nDelete application now? (y/n): ")

	pilihan, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(pilihan)) == "y" {
		fmt.Printf("\n-----> Menghapus %s...\n", packageName)
		exec.Command(adbPath, "uninstall", packageName).Run()

		// Verifikasi Akhir (Biar nggak nipu!)
		if isPackageStillExists(adbPath, packageName) {
			fmt.Println("[!] ERROR: Aplikasi gagal dihapus! Coba hapus manual via HP.\n[!] ERROR: Application failed to delete! Try deleting it manually via HP.")
		} else {
			fmt.Println("[OK] Konfirmasi: Aplikasi telah benar-benar terhapus.\n[OK] Confirmation: The application has been completely deleted.")
		}
	} else {
		fmt.Println("\n-----> Aplikasi dibiarkan terpasang.\n-----> Application left installed.")
	}

	tungguEnter(reader)
}

// --- FUNGSI HELPERS ---

func getPackageList(adbPath string) map[string]bool {
	list := make(map[string]bool)
	out, _ := exec.Command(adbPath, "shell", "pm", "list", "packages").Output()
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(strings.Replace(line, "package:", "", 1))
		if name != "" {
			list[name] = true
		}
	}
	return list
}

func findNewPackage(before, after map[string]bool) string {
	for pkg := range after {
		if !before[pkg] {
			return pkg
		}
	}
	return ""
}

func isPackageStillExists(adbPath, pkgName string) bool {
	out, _ := exec.Command(adbPath, "shell", "pm", "list", "packages", pkgName).Output()
	return strings.Contains(string(out), "package:"+pkgName)
}

func setupADB() string {
	base, _ := os.Getwd()
	adbFile := filepath.Join(base, engineDir, "platform-tools", "adb.exe")
	if _, err := os.Stat(adbFile); err == nil {
		return adbFile
	}

	fmt.Println("[*] Engine ADB tidak ditemukan. Mendownload...\n[*] ADB Engine not found. Downloading...")
	resp, _ := http.Get(adbURL)
	defer resp.Body.Close()
	f, _ := os.Create("adb.zip")
	io.Copy(f, resp.Body)
	f.Close()
	unzip("adb.zip", engineDir)
	os.Remove("adb.zip")
	return adbFile
}

func unzip(src, dest string) {
	r, _ := zip.OpenReader(src)
	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
		out, _ := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		rc, _ := f.Open()
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}
}

func tungguEnter(r *bufio.Reader) {
	fmt.Println("\nTekan Enter untuk keluar...\nPress Enter to exit...")
	r.ReadString('\n')
}
