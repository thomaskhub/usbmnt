package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	
	"github.com/tidwall/gjson"
)

type BlkDevT struct {
	Name    string
	Mount   string
	Symlink string
}

var knownDevices = make(map[string]*BlkDevT)

func InitMountDir() {
	//check if any director is set in MountPath, if so try to unmount and then delete
	files, err := ioutil.ReadDir(MountPath)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, file := range files {
		fmt.Println("Debug::Init::file found -->", file.Name())
		re, _ := regexp.MatchString("usb[0-9]*", file.Name())
		fmt.Println("Debg::Init::re ->", re)
		if re {
			path := MountPath + "/" + file.Name()
			symPath := SymlinkPath + "/" + file.Name()

			fmt.Println("Debg::Init::path ->", path)
			fmt.Println("Debg::Init::symPath ->", symPath)

			cmd := exec.Command("umount", path)
			cmd.Output()

			fmt.Println("Debg::Init::going to remove dir and link")
			os.Remove(path)
			os.Remove(symPath)
		}
	}
}

func handleAdd(dev string) {
	data, err := os.ReadFile("/etc/mtab")
	if err == nil {
		var re = regexp.MustCompile(`(?m)^/dev/` + dev)
		match := re.FindAllString(string(data), -1)
		if len(match) > 0 {
			return
		} else {
			files, err := ioutil.ReadDir(MountPath)
			if err != nil {
				fmt.Println(err)
				return
			}

			max := int64(-1)
			min := int64(1000)
			for _, file := range files {
				re, _ := regexp.MatchString("usb[0-9]*", file.Name())
				if re {
					id, _ := strconv.ParseInt(strings.Replace(file.Name(), "usb", "", -1), 10, 12)
					if id > max {
						max = id
					}
					if id < min {
						min = id
					}
				}
			}

			var nextUsbId int64
			if min > 0 && min < 10 {
				nextUsbId = min - 1
			} else {
				nextUsbId = max + 1
			}

			if nextUsbId < 10 {
				path := MountPath + "/usb" + fmt.Sprint(nextUsbId)
				err := os.Mkdir(path, os.ModePerm)
				if err != nil {
					fmt.Println(err)
					return
				}

				knownDevices[dev].Mount = path
				knownDevices[dev].Symlink = SymlinkPath + "/usb" + fmt.Sprint(nextUsbId)
				fmt.Println("Symlink --------------------", knownDevices[dev].Symlink)
				fmt.Println("Mount --------------------", knownDevices[dev].Mount)

				//mount the device, if we could not mount it we remove the directory agin. This can happen if
				//its an SDA device which has multiple
				cmd := exec.Command("mount", "-o", "sync,noexec,nodev,noatime,nodiratime", "/dev/"+dev, path)
				stdout, err := cmd.Output()
				if err != nil {
					fmt.Println("Error::Mount:", err, stdout)
					return
				}

				//TODO: implement automatic encryption from usb stick without admin privilges 
				// - check if the mounted USB drive contains a directory ending with regex _piimage_key[0-9]
				// - if yes unmount the drive and remount it rw
				// - then call http://localhost:8080/encrypt_connect - action=create|volume|keyName|encbase|init_size in body json
				// - this will create and open the image 
				// - once image is create and mounted (which should happen with the same command), 
				//   copy files to /mnt/Ishamedia/volume 
				// - once this is done close the volume http://localhost:8080/encrypt_connect action=close
				// - unmount the usb device 
				// - inform UI about: start of encrytion, end of encryption, error message if post calls fail
				// - once this is done mount the device as normal so that we can directly test is

				cmd = exec.Command("ln", "-s", knownDevices[dev].Mount, knownDevices[dev].Symlink)
				stdout, err = cmd.Output()
				if err != nil {
					fmt.Println("Error::Symlink:", err, stdout)
					return
				}

				// err = os.Symlink(knownDevices[dev].Mount, knownDevices[dev].Symlink)
				// fmt.Println(knownDevices[dev].Mount, knownDevices[dev].Symlink)
				// if err != nil { //if symlink was not possible just print error,we unmount when we remove the device
				// 	fmt.Println("Error::CreateSymlink::", err)
				// }

			} else {
				fmt.Println("Too many USB devices connected. We are not going to mount anymore usb devices")
			}

		}
	}

}

func handleRemove(dev string) {
	fmt.Println("Check check")
	fmt.Printf("%v", knownDevices)
	fmt.Println("Remove device --> ", dev, &knownDevices[dev].Mount)

	//make sure to only delete subdirectories but never the symlink dir
	if strings.HasPrefix(knownDevices[dev].Symlink, SymlinkPath + "/") {
		os.Remove(knownDevices[dev].Symlink)
	}
	fmt.Println(knownDevices[dev].Mount)
	//before we can unmount the device we need to make sure that the luks 
	//device is closed, otherwise it will cause issues  
	//1. losetup -l | grep /media/usb0 | awk '{print $1}' --> get the list of loop devices with mounted luks
	// each elemetn append p1 to get first partition and then call sudo e2label /dev/mapper/LOOPp1 which gives us the luks volume name 
	// call   
	//  sudo umount $MOUNT_PATH/$VOLNAME -- unmount the loop device with data 
	//	sudo cryptsetup luksClose $VOLNAME -- remove the volume 
	//   sudo kpartx -d -v $IMG_FILE -- delete partition dev mappings, after that we can unmount the /dev/sda device itself
	fmt.Println("going to close luks....", knownDevices[dev].Mount)
	closeLuksCmd := exec.Command("/bin/bash", "/opt/ishapi/closeVols.sh",knownDevices[dev].Mount)
	out,err := closeLuksCmd.Output()
	if err != nil {
		fmt.Println("Error::closeLuks::", err, out)
	}
	
	cmd := exec.Command("umount", knownDevices[dev].Mount)
	umountOut, err := cmd.Output()
	if err != nil {
		fmt.Println("Error::unmount::", err, umountOut)
	}
	
	//make sure to only delete the usb dir in mountpath, but not the mount path itself
	if strings.HasPrefix(knownDevices[dev].Mount, MountPath + "/") {
		os.Remove(knownDevices[dev].Mount)
	}
	delete(knownDevices, dev)
}

func ObserveBlockDev() []BlkDevT {
	files, _ := ioutil.ReadDir("/dev")
	var deviceNames []string
	curDevices := make(map[string]BlkDevT)

	for _, file := range files {
		re, _ := regexp.MatchString("sd[a-z][0-9]*$", file.Name())
		if re {
			cmd := exec.Command("lsblk", "/dev/"+file.Name(), "-J")
			stdout, err := cmd.Output()
			if err != nil {
				fmt.Println(err)
				continue
			}

			typ := gjson.Get(string(stdout), "blockdevices.0.type")
			mp := gjson.Get(string(stdout), "blockdevices.0.mountpoint")
			children := gjson.Get(string(stdout), "blockdevices.0.children")

			if typ.String() == "disk" && children.Exists() {
				continue
			}

			if typ.String() == "part" && children.Exists() {
				continue
			}

			if mp.Value() != nil && !strings.HasPrefix(mp.String(), MountPath) {
				// fmt.Println("Partition/Device already mounted by something else ignore this", file.Name(), mp.String())
				continue
			}

			deviceNames = append(deviceNames, file.Name())
			var device = BlkDevT{
				Name: file.Name(),
			}
			curDevices[file.Name()] = device
		}
	}

	//any key which is in know but not in cur means we can delete
	for key, _ := range knownDevices {
		_, found := curDevices[key]
		if !found {
			handleRemove(key)
		}
	}

	//any key which is in cur but not in known means new dev detected
	for key, value := range curDevices {
		_, found := knownDevices[key]
		if !found {
			knownDevices[key] = &BlkDevT{
				Name:    value.Name,
				Mount:   value.Mount,
				Symlink: value.Symlink,
			}
			// fmt.Printf("------------- %v", knownDevices[key].Name)
			handleAdd(key)
		}
	}

	return nil
}
