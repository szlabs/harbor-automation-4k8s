package utils

const (
	// AnnotationHarborServer is the annnotation for harbor server
	AnnotationHarborServer = "goharbor.io/harbor-server"
	// AnnotationAccount is the annnotation for service account
	AnnotationAccount = "goharbor.io/service-account"
	// AnnotationProject is the annnotation for harbor project name
	AnnotationProject = "goharbor.io/project"
	// AnnotationRewriter is the annotation for image rewrite
	AnnotationRewriter = "goharbor.io/image-rewrite"
	// ImageRewriteAuto is the mode of image rewrite
	// when set to auto, it will ensure project and robot for current namespace
	ImageRewriteAuto = "auto"
	// ImageRewriteGlobal is the mode of image rewrite
	// When set to global, it will ensure robot for current namespace. Throw error if project doesn't exist
	ImageRewriteGlobal = "global"
	// AnnotationRobot is the annotation for robot id
	AnnotationRobot = "goharbor.io/robot"
	// AnnotationRobotSecretRef is the annotation for robot secret reference
	AnnotationRobotSecretRef = "goharbor.io/robot-secret"
	// AnnotationSecOwner is the annotation for owner
	AnnotationSecOwner = "goharbor.io/owner"
)
