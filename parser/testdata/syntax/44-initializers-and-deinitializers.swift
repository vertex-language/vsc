// §6.7 Initializers and Deinitializers

class Device {
    var model: String
    var serial: String

    init(model: String, serial: String) {
        self.model = model
        self.serial = serial
    }

    convenience init(model: String) {
        self.init(model: model, serial: "UNKNOWN")
    }

    init?(validatedModel: String) {
        if validatedModel.isEmpty { return nil }
        self.model = validatedModel
        self.serial = "AUTO"
    }

    init!(legacyModel: String) {
        if legacyModel.isEmpty { return nil }
        self.model = legacyModel
        self.serial = "LEGACY"
    }

    deinit {
        print("Device \(model) deallocated")
    }
}

class Phone: Device {
    var carrier: String

    init(model: String, serial: String, carrier: String) {
        self.carrier = carrier
        super.init(model: model, serial: serial)
    }

    convenience init(carrier: String) {
        self.init(model: "DefaultPhone", serial: "000", carrier: carrier)
    }
}
