package uuid_service

type UUIDService struct{}

func New() *UUIDService {
	return &UUIDService{}
}

func (u *UUIDService) DeduplicateIDs(ids []string) []string {
	seen := make(map[string]bool)
	res := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			res = append(res, id)
		}
	}
	return res
}
