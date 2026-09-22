package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type MigrationCreator struct {
	serviceVersion string
	description    string
	directory      string
}

func NewMigrationCreator() *MigrationCreator {
	version := flag.String("v", "", "服务版本号 (例如: 1.9.4)")
	desc := flag.String("desc", "", "迁移描述")
	dir := flag.String("dir", "migrations", "迁移文件目录")
	flag.Parse()

	if *version == "" || *desc == "" {
		fmt.Println("请提供服务版本号和迁移描述")
		flag.Usage()
		os.Exit(1)
	}

	return &MigrationCreator{
		serviceVersion: *version,
		description:    *desc,
		directory:      *dir,
	}
}

func (m *MigrationCreator) validateInputs() error {
	versionPattern := `^\d+\.\d+\.\d+$`
	if match, _ := regexp.MatchString(versionPattern, m.serviceVersion); !match {
		return fmt.Errorf("版本号格式无效: %s, 应该类似于: 1.9.4", m.serviceVersion)
	}

	descPattern := `^[a-z][a-z0-9_]*$`
	if match, _ := regexp.MatchString(descPattern, m.description); !match {
		return fmt.Errorf("描述格式无效: %s, 只能包含小写字母、数字和下划线，且必须以字母开头", m.description)
	}

	return nil
}

func (m *MigrationCreator) generateFileName() string {
	version := strings.ReplaceAll(m.serviceVersion, ".", "_")
	return fmt.Sprintf("v%s_%s", version, m.description)
}

func (m *MigrationCreator) createMigrationFiles() error {
	if err := os.MkdirAll(m.directory, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	fileName := m.generateFileName()

	cmd := exec.Command("migrate",
		"create",
		"-ext", "sql",
		"-dir", m.directory,
		fileName)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("创建迁移文件失败: %v\n%s", err, output)
	}

	fmt.Printf("成功创建迁移文件:\n%s", output)
	return nil
}

func main() {
	creator := NewMigrationCreator()

	if err := creator.validateInputs(); err != nil {
		log.Fatalf("输入验证失败: %v", err)
	}

	if err := creator.createMigrationFiles(); err != nil {
		log.Fatalf("创建迁移文件失败: %v", err)
	}
}
