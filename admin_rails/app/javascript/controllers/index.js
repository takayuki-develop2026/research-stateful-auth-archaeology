// Import and register all your controllers from the importmap via controllers/**/*_controller
import { application } from "controllers/application";
import { eagerLoadControllersFrom } from "@hotwired/stimulus-loading";
eagerLoadControllersFrom("controllers", application);

import DashCardController from "./dash_card_controller";
window.Stimulus = Application.start();
Stimulus.register("dash-card", DashCardController);