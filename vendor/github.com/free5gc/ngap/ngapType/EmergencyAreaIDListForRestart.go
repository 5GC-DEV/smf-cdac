package ngapType

// Need to import "github.com/free5gc/aper" if it uses "aper"

/* Sequence of = 35, FULL Name = struct EmergencyAreaIDListForRestart */
/* EmergencyAreaID */
type EmergencyAreaIDListForRestart struct {
	List []EmergencyAreaID `aper:"sizeLB:1,sizeUB:256"`
}
