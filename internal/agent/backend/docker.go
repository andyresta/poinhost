package backend

import "github.com/andyresta/poinhost/internal/modules/docker"

// Containers membaca seluruh container Docker di mesin ini.
func (b *Backend) Containers() (ContainerListing, error) {
	if b.selfID == "" || b.docker == nil {
		return ContainerListing{}, errBelumKelolaDiri
	}
	res, err := b.docker.ListContainers(b.selfID)
	if err != nil {
		return ContainerListing{}, err
	}
	out := ContainerListing{DockerOK: res.DockerOK, Version: res.Version}
	out.Containers = make([]ContainerInfo, 0, len(res.Containers))
	for _, c := range res.Containers {
		out.Containers = append(out.Containers, ContainerInfo{
			ID:      c.ID,
			Name:    c.Name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Ports:   c.Ports,
			Created: c.Created,
		})
	}
	return out, nil
}

// ContainerLogs membaca ekor log satu container.
//
// Hanya ekornya, tidak pernah seluruhnya, dan batasnya ditegakkan di sini:
// `docker logs` tanpa --tail bisa mengembalikan ratusan megabita, yang akan
// ditahan di memori agent sebelum sempat dikirim.
func (b *Backend) ContainerLogs(id string, lines int) (string, error) {
	if b.selfID == "" || b.docker == nil {
		return "", errBelumKelolaDiri
	}
	if lines <= 0 || lines > maxBarisLog {
		lines = maxBarisLog
	}
	res, err := b.docker.ContainerLogs(docker.LogsRequest{
		ServerID:    b.selfID,
		ContainerID: id,
		Lines:       lines,
	})
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// maxBarisLog adalah batas atas baris log yang boleh diminta sekali.
const maxBarisLog = 2000

// ContainerAction menjalankan satu aksi pada satu container.
//
// Validasi id container ada di modul docker, sama seperti yang dipakai app
// desktop — bukan disalin ke sini.
func (b *Backend) ContainerAction(id, action string) error {
	if b.selfID == "" || b.docker == nil {
		return errBelumKelolaDiri
	}
	switch action {
	case "start":
		return b.docker.StartContainer(b.selfID, id)
	case "stop":
		return b.docker.StopContainer(b.selfID, id)
	case "restart":
		return b.docker.RestartContainer(b.selfID, id)
	}
	// Tidak akan tercapai: lapisan API sudah menyaringnya lebih dulu.
	// Tetap ada supaya aksi baru yang lupa disaring di sana gagal di sini,
	// bukan diam-diam tidak melakukan apa pun.
	return errAksiTidakDikenal
}
