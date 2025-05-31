package sevenz

import (
	"bytes"
	"github.com/sirupsen/logrus"
	"io"
	"strings"
	"testing"
)

const (
	testPathToISO1 = "D:\\Distribution\\ISO\\Astra Linux\\AstraLinuxPackage_1.0.0\\smolensk-1.6\\smolensk-1.6-20.06.2018_15.52.iso"
	testPathTo7z   = "C:\\Program Files\\7-Zip\\7z.exe"
)

func TestSevenZ_find7ZBin(t *testing.T) {
	type fields struct {
		log     *logrus.Entry
		ISOPath string
		Version string
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		{
			name:    "Найдены",
			fields:  fields{},
			want:    testPathTo7z,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := SevenZ{
				log:     tt.fields.log,
				ISOPath: tt.fields.ISOPath,
				Version: tt.fields.Version,
			}
			got, err := m.find7ZBin()
			if (err != nil) != tt.wantErr {
				t.Errorf("find7ZBin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !strings.EqualFold(got, tt.want) {
				t.Errorf("find7ZBin() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSevenZ_exec7zOnce(t *testing.T) {
	log := logrus.New()
	log.Out = io.Discard

	type fields struct {
		log        *logrus.Entry
		ISOPath    string
		Version    string
		sevenZPath string
	}
	type args struct {
		args []string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Неправильный аргумент",
			fields: fields{
				log:        log.WithField("a", "b"),
				sevenZPath: testPathTo7z,
			},
			args: args{
				args: []string{"aaa"},
			},
			want:    "7-Zip 22.01 (x64) : Copyright (c) 1999-2022 Igor Pavlov : 2022-07-15\n\n\n\nCommand Line Error:\nUnsupported command:\naaa",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := SevenZ{
				log:        tt.fields.log,
				ISOPath:    tt.fields.ISOPath,
				Version:    tt.fields.Version,
				sevenZPath: tt.fields.sevenZPath,
			}
			got, err := m.exec7zOnce(tt.args.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("exec7zOnce() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if strings.EqualFold(got, tt.want) {
				t.Errorf("exec7zOnce() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSevenZ_readFileOut(t *testing.T) {
	log := logrus.New()
	log.Out = io.Discard

	type fields struct {
		log        *logrus.Entry
		ISOPath    string
		Version    string
		sevenZPath string
	}
	type args struct {
		file string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantOut string
		wantErr bool
	}{
		{
			name: "",
			fields: fields{
				log:        log.WithField("a", "b"),
				ISOPath:    testPathToISO1,
				sevenZPath: testPathTo7z,
			},
			args: args{
				file: ".disk/info",
			},
			wantOut: "OS Astra Linux 1.6 smolensk - amd64 DVD",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &SevenZ{
				log:        tt.fields.log,
				ISOPath:    tt.fields.ISOPath,
				Version:    tt.fields.Version,
				sevenZPath: tt.fields.sevenZPath,
			}
			out := &bytes.Buffer{}
			err := m.ReadFile(tt.args.file, out)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFile() error = `%v`, wantErr `%v`", err, tt.wantErr)
				return
			}
			if gotOut := out.String(); strings.TrimSpace(gotOut) != tt.wantOut {
				t.Errorf("ReadFile() gotOut = `%v`, want `%v`", gotOut, tt.wantOut)
			}
		})
	}
}
