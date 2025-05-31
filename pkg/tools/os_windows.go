package tools

import (
	"github.com/gonutz/w32/v2"
)

// OsVersion возвращает тип операционной сиситемы (windows|linux) и её версию (major, minor, patch).
//
//	| ------------------------------ | ------------- |
//	| Операционная система           | номер версии; |
//	| ------------------------------ | ------------- |
//	| Windows 11                     | 10.0*         |
//	| Windows 10                     | 10.0*         |
//	| Windows Server 2022            | 10.0*         |
//	| Windows Server 2019            | 10.0*         |
//	| Windows Server 2016            | 10.0*         |
//	| Windows 8.1                    | 6.3*          |
//	| Windows Server 2012 R2         | 6.3*          |
//	| Windows 8                      | 6.2           |
//	| Windows Server 2012            | 6.2           |
//	| Windows 7                      | 6.1           |
//	| Windows Server 2008 R2         | 6.1           |
//	| Windows Server 2008            | 6,0           |
//	| Windows Vista                  | 6,0           |
//	| Windows Server 2003 R2         | 5,2           |
//	| Windows Server 2003            | 5,2           |
//	| Windows XP 64-разрядная версия | 5,2           |
//	| Windows XP                     | 5.1           |
//	| Windows 2000                   | 5,0           |
//	| ------------------------------ | ------------- |
func OsVersion() (string, int, int, int) {
	version := w32.GetVersion()
	major, minor := version&0xFF, version&0xFF00>>8
	return "windows", int(major), int(minor), 0
}
