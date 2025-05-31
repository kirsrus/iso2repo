package sevenz

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"testing"

	"github.com/juju/errors"
	"github.com/maxatome/go-testdeep/td"
	"github.com/sirupsen/logrus"
)

func TestSevenZ_readFilesInISO_testRegexp(t *testing.T) {
	t.Run("Тест папок и файлов", func(t *testing.T) {
		reFile := regexp.MustCompile(`^(\d+-\d+-\d+)\s+(\d+:\d+:\d+)\s+(.|D)\.\.\.\.\s+(\d*)\s+(\d*)\s+([^ ]+)$`)

		type pattern struct {
			str       string
			resultStr []string
			wantError bool
		}

		lines := []pattern{
			{
				"2018-06-20 18:50:00 .....         5188         5188  pool\\non-free\\libp\\libparsec-mac-qt5\\libparsec-mac-qt5-1-dev_0.13.8_amd64.deb",
				[]string{"2018-06-20", "18:50:00", ".", "5188", "5188", "pool\\non-free\\libp\\libparsec-mac-qt5\\libparsec-mac-qt5-1-dev_0.13.8_amd64.deb"},
				false,
			},
			{
				"2018-06-20 20:32:10 .....            0            0  astra",
				[]string{"2018-06-20", "20:32:10", ".", "0", "0", "astra"},
				false,
			},
			{
				"2022-07-22 12:33:04 .....         4766         6144  info@axxonsoft.com.gpg.key",
				[]string{"2022-07-22", "12:33:04", ".", "4766", "6144", "info@axxonsoft.com.gpg.key"},
				false,
			},
			{
				"2022-04-21 15:16:51 .....   5306496802   5306497024  pool\\main\\a\\axxon-detector-pack\\axxon-detector-pack_3.7.4.85_amd64.deb",
				[]string{"2022-04-21", "15:16:51", ".", "5306496802", "5306497024", "pool\\main\\a\\axxon-detector-pack\\axxon-detector-pack_3.7.4.85_amd64.deb"},
				false,
			},
			{
				"2018-06-20 20:32:10 D....                            boot",
				[]string{"2018-06-20", "20:32:10", "D", "", "", "boot"},
				false,
			},
			{
				"2018-06-20 20:52:22 D....                            boot\\grub\\i386-efi",
				[]string{"2018-06-20", "20:52:22", "D", "", "", "boot\\grub\\i386-efi"},
				false,
			},
			{
				"2022-07-22 12:33:04         5916878426   5917296640  407 files, 91 folders",
				[]string{},
				true,
			},
		}

		for _, line := range lines {
			match := reFile.FindStringSubmatch(line.str)

			if line.wantError {
				if len(match) != 0 {
					t.Errorf("нашлось, что не должно находиться %#v", match)
					continue
				}
			}

			if len(match) == 0 {
				t.Errorf("не распознана линия: %s", line.resultStr)
			} else if len(match)-1 != len(line.resultStr) {
				t.Errorf("want: %d массив, got: %d массв", len(line.resultStr), len(match)-1)
			} else {
				for idx, e := range line.resultStr {
					if match[idx+1] != e {
						t.Errorf("want: %s\ngot:%s", e, match[idx+1])
					}
				}
			}
		}
	})

}

func Test_recurseFileAdd(t *testing.T) {
	root := NewRoot()
	root2 := NewRoot()

	root3 := NewRoot()
	root3.Children = append(root3.Children, File{
		IsDir:    true,
		Children: make([]File, 0),
		Name:     "a",
	})
	root3.Children[0].Children = append(root3.Children[0].Children, File{
		IsDir:    true,
		Children: make([]File, 0),
		Name:     "b",
	})
	root3.Children[0].Children[0].Children = append(root3.Children[0].Children[0].Children, File{
		IsDir:    false,
		Children: make([]File, 0),
		Name:     "c_exist",
	})

	type args struct {
		parts  []File
		pos    int
		result *File
	}
	tests := []struct {
		name string
		args args
		want *File
	}{
		{
			name: "линейные 2 директории и файл",
			args: args{
				parts:  []File{{IsDir: true, Name: "a"}, {IsDir: true, Name: "b"}, {IsDir: false, Name: "c"}},
				pos:    0,
				result: &root,
			},
			want: &File{
				IsRoot: true,
				IsDir:  true,
				Children: []File{
					{
						IsDir: true,
						Name:  "a",
						Children: []File{
							{
								IsDir: true,
								Name:  "b",
								Children: []File{
									{
										Name:     "c",
										Children: make([]File, 0),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "линейные 3 директории",
			args: args{
				parts:  []File{{IsDir: true, Name: "a"}, {IsDir: true, Name: "b"}, {IsDir: true, Name: "c"}},
				pos:    0,
				result: &root2,
			},
			want: &File{
				IsRoot: true,
				IsDir:  true,
				Children: []File{
					{
						IsDir: true,
						Name:  "a",
						Children: []File{
							{
								IsDir: true,
								Name:  "b",
								Children: []File{
									{
										IsDir:    true,
										Name:     "c",
										Children: make([]File, 0),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "линейные 3 директории с существующими дир. и файлами",
			args: args{
				parts:  []File{{IsDir: true, Name: "a"}, {IsDir: true, Name: "b"}, {IsDir: true, Name: "c"}},
				pos:    0,
				result: &root3,
			},
			want: &File{
				IsRoot: true,
				IsDir:  true,
				Children: []File{
					{
						IsDir: true,
						Name:  "a",
						Children: []File{
							{
								IsDir: true,
								Name:  "b",
								Children: []File{
									{
										IsDir:    false,
										Name:     "c_exist",
										Children: make([]File, 0),
									},
									{
										IsDir:    true,
										Name:     "c",
										Children: make([]File, 0),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recurseFileAdd(tt.args.parts, tt.args.pos, &tt.args.result.Children)

			td.Cmp(t, tt.want, tt.args.result)
		})
	}
}

// Тест чтения репозиториев в TGZ-файлах
func TestSevenZ_readTar(t *testing.T) {

	// Тестируемый tgz репозиторий
	tgzFilePath := "D:\\Distribution\\ISO\\Astra Linux\\AstraLinuxPackage_3.0.0\\smolensk-1.7.3\\base-1.7.3-03.11.2022_15.53.tar"

	if _, err := os.Stat(tgzFilePath); os.IsNotExist(err) {
		t.Fatalf("tgz файл \"%s\" не найден", tgzFilePath)
	}

	log := logrus.New()
	log.Out = io.Discard

	sz, err := NewSevenZ(tgzFilePath, log)
	if err != nil {
		t.Fatalf(errors.ErrorStack(err))
	}

	releaseBuff := new(bytes.Buffer)
	err = sz.ReadFile("dists\\1.7_x86-64\\Release.gpg", releaseBuff)
	if err != nil {
		t.Fatal(errors.ErrorStack(err))
	} else if releaseBuff.Len() == 0 {
		t.Fatal("буфер не может быть пуст; значит файл не прочитан")
	}

}
